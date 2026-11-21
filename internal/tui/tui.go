// Package tui is the Bubble Tea app: tabs on top, one pane, a status and prompt line at the bottom.
// The same model runs locally and over SSH.
package tui

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/dbabi/agentbox-web/content"
	"github.com/dbabi/agentbox-web/internal/snib"
)

// MainCommand is what c copies from any tab.
const MainCommand = content.MainCommand

const (
	maxInput   = 64
	msgTTL     = 2500 * time.Millisecond
	idleBye    = 110 * time.Second // the sleeping-Snib goodbye, 10 s before the cutoff
	idleCutoff = 10 * time.Second  // after the goodbye
	minW, minH = 40, 12
	artW, artH = 32, 16
	artAboveH  = 36       // below this height the art above would push the text off an 80x24 screen
	maxW, maxH = 500, 300 // the client sets the size; View allocates width x height, so cap it
	status     = content.Status
)

// Snib palette, from mascot/README.md in dbabi/agentbox.
var (
	ink       = lipgloss.Color("#2a1f45")
	highlight = lipgloss.Color("#f6f2ff")
	light     = lipgloss.Color("#cfc3ff")
	body      = lipgloss.Color("#a48df5")
	shade     = lipgloss.Color("#7259d1")
	deep      = lipgloss.Color("#5a43b0")
	gold      = lipgloss.Color("#ffcc4d")
	goldShade = lipgloss.Color("#e0902f")
	mint      = lipgloss.Color("#7cf5d0")
	blush     = lipgloss.Color("#ff8fb8")
)

var (
	st     = lipgloss.NewStyle
	bar    = st().Background(ink).Foreground(highlight)
	barDim = st().Background(ink).Foreground(light)
	barGld = st().Background(ink).Foreground(gold)
	barSep = st().Background(ink).Foreground(shade)
	active = st().Background(ink).Foreground(gold).Underline(true)
	cursor = st().Background(deep).Foreground(highlight)
	kindSt = map[content.Kind]lipgloss.Style{
		content.Comment: st().Foreground(light),
		content.Caution: st().Foreground(goldShade),
		content.Pre:     st().Foreground(light),
	}
	tagSt = map[string]lipgloss.Style{
		"built": st().Foreground(mint), "passed": st().Foreground(mint), "ok": st().Foreground(mint),
		"next": st().Foreground(gold), "partly built": st().Foreground(gold), "in progress": st().Foreground(gold),
	}
	textSt = st().Foreground(highlight)
	linkSt = st().Foreground(body).Underline(true)
	errSt  = st().Foreground(blush)
	okSt   = st().Foreground(mint)
	tagRe  = regexp.MustCompile(`\[(built|partly built|designed|in progress|ok|--|passed|next|planned|not run)\]`)
)

// Keys is the keymap. The help bar and the ? overlay are both generated from it.
type Keys struct {
	Next, Prev, Down, Up, Top, Bottom, HalfDown, HalfUp, PageDown, Enter, Copy, Prompt, Help, Motion, Quit key.Binding
}

// DefaultKeys is the spec keymap.
var DefaultKeys = Keys{
	Next:     key.NewBinding(key.WithKeys("l", "right", "]", "tab"), key.WithHelp("l/→/]/tab", "next tab")),
	Prev:     key.NewBinding(key.WithKeys("h", "left", "[", "shift+tab"), key.WithHelp("h/←/[", "previous tab")),
	Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/k", "move")),
	Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
	Top:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g/G", "top, bottom")),
	Bottom:   key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
	HalfDown: key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("ctrl+d/pgdn", "half page down")),
	HalfUp:   key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("ctrl+u/pgup", "half page up")),
	PageDown: key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "page down")),
	Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "copy")),
	Copy:     key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "copy go run")),
	Prompt:   key.NewBinding(key.WithKeys("/", ":"), key.WithHelp("/", "prompt")),
	Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Motion:   key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "motion")),
	Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// ShortHelp is the status-bar hint list.
func (k Keys) ShortHelp() []key.Binding { return []key.Binding{k.Down, k.Enter, k.Prompt, k.Help} }

// FullHelp is the ? overlay.
func (k Keys) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{key.NewBinding(key.WithKeys("1"), key.WithHelp("1-7", "jump to a tab")), k.Next, k.Prev},
		{k.Down, k.Up, k.Top, k.HalfDown, k.HalfUp, k.PageDown},
		{k.Enter, k.Copy, k.Prompt, k.Help, k.Motion, k.Quit},
	}
}

// Options set how a Model starts.
type Options struct {
	Page    string // start tab slug or key; "" is home
	Motion  bool   // play the idle loop on the snib tab
	SSH     bool   // over SSH: idle goodbye and cutoff
	Profile colorprofile.Profile
}

// Model is the one Bubble Tea model for both modes.
type Model struct {
	pages              []content.Page
	tab, cursor, top   int
	width, height      int
	ascii, motion, ssh bool
	ticking            bool
	frame              int
	prompting, help    bool
	input              []rune
	msg                string
	msgErr             bool
	msgSeq, idleSeq    int
	bye                bool
	keys               Keys
	helpM              help.Model
}

type (
	frameMsg struct{}
	clearMsg struct{ seq int }
	idleMsg  struct{ seq int }
	quitMsg  struct{ seq int }
)

// New builds a model. It fails only if the embedded content does not parse.
func New(o Options) (Model, error) {
	pages, err := content.Pages()
	if err != nil {
		return Model{}, err
	}
	m := Model{pages: pages, motion: o.Motion, ssh: o.SSH, width: 80, height: 24, keys: DefaultKeys, helpM: help.New()}
	m.setProfile(o.Profile)
	if o.Page != "" {
		i := m.find(o.Page)
		if i < 0 {
			return m, fmt.Errorf("no such page: %s", o.Page)
		}
		m.tab = i
	}
	m.home()
	return m, nil
}

func (m *Model) setProfile(p colorprofile.Profile) {
	m.ascii = p <= colorprofile.ASCII
	hs := help.Styles{
		ShortKey: barGld, ShortDesc: barDim, ShortSeparator: barDim, Ellipsis: barDim,
		FullKey: st().Foreground(gold), FullDesc: st().Foreground(highlight), FullSeparator: st().Foreground(light),
	}
	m.helpM.Styles = hs
}

func (m Model) find(name string) int {
	name = strings.TrimSpace(name)
	for i, p := range m.pages {
		if p.Slug == name || fmt.Sprint(p.Key) == name {
			return i
		}
	}
	return -1
}

// Init starts the idle loop and the SSH idle timer.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.ssh {
		cmds = append(cmds, idleTick(m.idleSeq))
	}
	if m.wantsFrames() {
		cmds = append(cmds, frameTick())
	}
	return tea.Batch(cmds...)
}

func frameTick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return frameMsg{} })
}
func idleTick(seq int) tea.Cmd {
	return tea.Tick(idleBye, func(time.Time) tea.Msg { return idleMsg{seq} })
}

func (m Model) wantsFrames() bool { return m.motion && m.page().Pose == "all" }

func (m Model) page() content.Page { return m.pages[m.tab] }

// Update handles keys, pastes, resizes and ticks.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(0, min(msg.Width, maxW)), max(0, min(msg.Height, maxH))
		m.home() // the layout, and so the row list, depends on the size
	case tea.ColorProfileMsg:
		m.setProfile(msg.Profile)
	case frameMsg:
		if !m.wantsFrames() {
			m.ticking = false
			return m, nil
		}
		m.frame = (m.frame + 1) % 4
		return m, frameTick()
	case clearMsg:
		if msg.seq == m.msgSeq {
			m.msg = ""
		}
	case idleMsg:
		if msg.seq == m.idleSeq {
			m.bye = true
			return m, tea.Tick(idleCutoff, func(time.Time) tea.Msg { return quitMsg(msg) })
		}
	case quitMsg:
		if msg.seq == m.idleSeq {
			return m, tea.Quit
		}
	case tea.PasteMsg:
		var cmd tea.Cmd
		if m.prompting {
			if !m.typeText(msg.Content) {
				cmd = m.say(fmt.Sprintf("# paste cut to %d characters", maxInput), true)
			}
		}
		return m.touched(cmd)
	case tea.KeyPressMsg:
		m.bye = false
		var cmd tea.Cmd
		if m.prompting {
			cmd = m.promptKey(msg)
		} else {
			cmd = m.key(msg)
		}
		return m.touched(cmd)
	}
	return m, nil
}

// touched restarts the SSH idle timer and the idle loop, if needed, after any input.
func (m Model) touched(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	cmds := []tea.Cmd{cmd}
	if m.ssh {
		m.idleSeq++
		cmds = append(cmds, idleTick(m.idleSeq))
	}
	if m.wantsFrames() && !m.ticking {
		m.ticking = true
		cmds = append(cmds, frameTick())
	}
	return m, tea.Batch(cmds...)
}

func (m *Model) say(s string, isErr bool) tea.Cmd {
	m.msg, m.msgErr = s, isErr
	m.msgSeq++
	seq := m.msgSeq
	return tea.Tick(msgTTL, func(time.Time) tea.Msg { return clearMsg{seq} })
}

func (m *Model) go2(i int) {
	m.tab = (i + len(m.pages)) % len(m.pages)
	m.help = false
	m.home()
}

func (m *Model) key(k tea.KeyPressMsg) tea.Cmd {
	s := k.String()
	if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
		if i := m.find(s); i >= 0 {
			m.go2(i)
		}
		return nil
	}
	ks := m.keys
	rows := m.rows()
	ph := m.paneHeight()
	switch {
	case s == "esc":
		m.help = false
	case key.Matches(k, ks.Quit):
		return tea.Quit
	case key.Matches(k, ks.Next):
		m.go2(m.tab + 1)
	case key.Matches(k, ks.Prev):
		m.go2(m.tab - 1)
	case key.Matches(k, ks.Down):
		m.move(1)
	case key.Matches(k, ks.Up):
		m.move(-1)
	case key.Matches(k, ks.Top):
		m.home()
	case key.Matches(k, ks.Bottom):
		m.move(len(rows))
	case key.Matches(k, ks.HalfDown):
		m.move(ph / 2)
	case key.Matches(k, ks.HalfUp):
		m.move(-ph / 2)
	case key.Matches(k, ks.PageDown):
		m.move(ph)
	case key.Matches(k, ks.Enter):
		if m.cursor < len(rows) {
			r := rows[m.cursor]
			// OSC 52 fails silently in many terminals, so the message always shows the text to select by hand
			switch {
			case r.url != "":
				return tea.Batch(tea.SetClipboard(r.url), m.say("# copy: "+r.url, false))
			case r.cmd != "":
				return tea.Batch(tea.SetClipboard(r.cmd), m.say("# copy: "+r.cmd, false))
			}
		}
	case key.Matches(k, ks.Copy):
		return tea.Batch(tea.SetClipboard(MainCommand), m.say("# copy: "+MainCommand, false))
	case key.Matches(k, ks.Prompt):
		m.prompting = true
	case key.Matches(k, ks.Help):
		m.help = !m.help
	case key.Matches(k, ks.Motion):
		m.motion = !m.motion
		if m.motion {
			return m.say("# motion on", false)
		}
		return m.say("# motion off", false)
	}
	return nil
}

func (m *Model) promptKey(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc":
		m.prompting, m.input, m.help = false, nil, false
	case "backspace":
		if n := len(m.input); n > 0 {
			m.input = m.input[:n-1]
		}
	case "ctrl+u":
		m.input = nil
	case "tab":
		m.complete()
	case "enter":
		line := string(m.input)
		m.prompting, m.input = false, nil
		return m.run(line)
	default:
		m.typeText(k.Text)
	}
	return nil
}

// typeText appends printable runes only, up to maxInput. It reports false if it had to cut s.
func (m *Model) typeText(s string) bool {
	for _, r := range s {
		if !unicode.IsPrint(r) {
			continue
		}
		if len(m.input) >= maxInput {
			return false
		}
		m.input = append(m.input, r)
	}
	return true
}

func (m *Model) complete() {
	f := strings.Fields(string(m.input))
	if len(f) == 0 || len(f) > 2 {
		return
	}
	word, pre := f[len(f)-1], ""
	if len(f) == 2 {
		pre = f[0] + " "
	}
	for _, p := range m.pages {
		if strings.HasPrefix(p.Slug, word) {
			m.input = []rune(pre + p.Slug)
			return
		}
	}
}

// run executes one prompt line. Output is a fixed string or a page name, never raw terminal bytes.
func (m *Model) run(line string) tea.Cmd {
	f := strings.Fields(line)
	if len(f) == 0 {
		return nil
	}
	arg := f[0]
	if (f[0] == "cd" || f[0] == "cat") && len(f) == 2 {
		arg = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(f[1], "~/"), "~"), "/")
		if arg == "" {
			arg = "home"
		}
	} else if len(f) != 1 && f[0] != "keys" {
		arg = ""
	}
	switch {
	case f[0] == "help":
		m.help = true
	case f[0] == "ls":
		var s []string
		for _, p := range m.pages {
			s = append(s, p.Slug)
		}
		return m.say("# "+strings.Join(s, "  "), false)
	case f[0] == "clear":
		m.msg = ""
	case f[0] == "keys":
		return m.say("# keys on|off is for the web. the terminal keeps its keys", false)
	case m.find(arg) >= 0:
		m.go2(m.find(arg))
	default:
		return m.say("# command not found: "+line+". try help", true)
	}
	return nil
}

// row is one rendered pane line.
type row struct {
	kind     content.Kind
	text     string // plain text, with its marker
	styled   string // set for art and swatches: already styled
	url, cmd string
	link     string // the link text inside text, if any
}

func (m Model) wrapWidth() int {
	w := min(m.width-4, 76)
	if m.artSide() {
		w = min(m.width-4-artW-2, 76)
	}
	return max(w, 10)
}

func (m Model) artSide() bool  { return m.width >= 100 && m.height >= 24 }
func (m Model) artAbove() bool { return !m.artSide() && m.width >= 60 && m.height >= artAboveH }

func (m Model) pose() (string, int) {
	p := m.page().Pose
	if p == "all" {
		return "idle", m.frame
	}
	return p, 0
}

// rows renders the current page into wrapped rows. With art above, the art scrolls with the text.
func (m Model) rows() []row {
	if m.help {
		return m.helpRows()
	}
	var out []row
	if m.artAbove() {
		pose, f := m.pose()
		for _, l := range art(pose, f, m.ascii) {
			out = append(out, row{kind: content.Pre, styled: l})
		}
		out = append(out, row{kind: content.Blank})
	}
	w := m.wrapWidth()
	for _, l := range m.page().Lines {
		out = append(out, lineRows(l, w)...)
	}
	if m.page().Pose == "all" && m.width >= minW {
		for _, pose := range []string{"happy", "thinking", "guarding", "sleeping"} {
			out = append(out, row{kind: content.Blank}, row{kind: content.Comment, text: "# " + pose})
			for _, l := range art(pose, 0, m.ascii) {
				out = append(out, row{kind: content.Pre, styled: l})
			}
		}
	}
	return out
}

func (m Model) helpRows() []row {
	out := []row{{kind: content.Comment, text: "# keys. esc closes this."}, {kind: content.Blank}}
	view := m.helpM.FullHelpView(m.keys.FullHelp())
	if lipgloss.Width(view)+4 > m.width { // too narrow for the columns side by side: stack them
		var groups []string
		for _, g := range m.keys.FullHelp() {
			groups = append(groups, m.helpM.FullHelpView([][]key.Binding{g}))
		}
		view = strings.Join(groups, "\n\n")
	}
	for _, l := range strings.Split(view, "\n") {
		out = append(out, row{kind: content.Pre, styled: l})
	}
	out = append(out, row{kind: content.Blank})
	hint := content.Line{Kind: content.Comment, Spans: []content.Span{{Text: "prompt: home, cd roadmap, cat install, ls, help, clear, snib. ctrl+u clears it."}}}
	return append(out, lineRows(hint, m.wrapWidth())...)
}

func lineRows(l content.Line, w int) []row {
	var text, url, link strings.Builder
	for _, s := range l.Spans {
		text.WriteString(s.Text)
		if s.URL != "" && url.Len() == 0 {
			url.WriteString(s.URL)
			link.WriteString(s.Text)
		}
	}
	base := row{kind: l.Kind, url: url.String(), link: link.String()}
	switch l.Kind {
	case content.Blank:
		return []row{base}
	case content.Pre:
		var out []row
		for _, r := range l.Raw {
			out = append(out, row{kind: content.Pre, text: r})
		}
		return out
	case content.Swatch:
		base.text = "██ " + text.String()
		base.styled = st().Foreground(lipgloss.Color(l.Hex)).Render("██") + " " + textSt.Render(text.String())
		return []row{base}
	case content.Command:
		base.cmd = text.String()
		base.text = "$ " + base.cmd // commands are never wrapped, so they copy as one line
		return []row{base}
	}
	first, rest := "", ""
	if tag, phases, rest0, ok := l.Status(); ok {
		// status rows keep their columns: continuation lines hang under the text
		first = tag + " "
		if phases != "" {
			first = fmt.Sprintf("%-10s%-6s", tag, phases)
		}
		rest = strings.Repeat(" ", len(first))
		text.Reset()
		text.WriteString(rest0)
	}
	switch l.Kind {
	case content.Comment:
		first, rest = "# ", "# "
	case content.Caution:
		first, rest = "# CAUTION: ", "# "
	case content.Bullet:
		first, rest = "- ", "  "
	case content.Step:
		first = fmt.Sprintf("%d. ", l.N)
		rest = strings.Repeat(" ", len(first))
	}
	var out []row
	for i, t := range wrap(first, rest, text.String(), w) {
		r := base
		r.text = t
		if i > 0 {
			r.link = "" // the link text sits on the first row
		}
		out = append(out, r)
	}
	return out
}

// wrap breaks s into lines of at most w cells. Status tags are never split.
func wrap(first, rest, s string, w int) []string {
	s = tagRe.ReplaceAllStringFunc(s, func(t string) string { return strings.ReplaceAll(t, " ", "\x00") })
	var out []string
	line, pre := first, first
	for _, word := range strings.Fields(s) {
		if len(line) > len(pre) && ansi.StringWidth(line)+1+ansi.StringWidth(word) > w {
			out = append(out, line)
			line, pre = rest, rest
		}
		if len(line) > len(pre) {
			line += " "
		}
		line += word
	}
	out = append(out, line)
	for i := range out {
		out[i] = strings.ReplaceAll(out[i], "\x00", " ")
	}
	return out
}

func (r row) render(w int, isCursor bool) string {
	if r.styled != "" && !isCursor {
		return r.styled
	}
	text := r.text
	if r.styled != "" {
		text = ansi.Strip(r.styled)
	}
	if isCursor {
		pad := max(0, w-ansi.StringWidth(text))
		return cursor.Render(text + strings.Repeat(" ", pad))
	}
	var s string
	switch r.kind {
	case content.Command:
		s = st().Foreground(body).Render("$ ") + textSt.Render(r.cmd)
	default:
		base, ok := kindSt[r.kind]
		if !ok {
			base = textSt
		}
		s = tags(text, base)
	}
	if r.link != "" {
		if i := strings.Index(text, r.link); i >= 0 {
			// OSC 8 makes the link text clickable in terminals that support it; the URL is fixed content.
			linked := ansi.SetHyperlink(r.url) + linkSt.Render(r.link) + ansi.ResetHyperlink()
			s = tags(text[:i], textSt) + linked + tags(text[i+len(r.link):], textSt)
		}
	}
	return s
}

// tags styles text with base, and each status tag in its own color. The tag text stays, so color is not the only cue.
func tags(text string, base lipgloss.Style) string {
	var b strings.Builder
	last := 0
	for _, ix := range tagRe.FindAllStringSubmatchIndex(text, -1) {
		b.WriteString(base.Render(text[last:ix[0]]))
		ts, ok := tagSt[text[ix[2]:ix[3]]]
		if !ok {
			ts = st().Foreground(light)
		}
		b.WriteString(ts.Render(text[ix[0]:ix[1]]))
		last = ix[1]
	}
	if last < len(text) {
		b.WriteString(base.Render(text[last:]))
	}
	return b.String()
}

func (m Model) paneHeight() int { return max(1, m.height-2) }

func (m *Model) home() {
	m.top, m.cursor = 0, 0
	rows := m.rows()
	for m.cursor < len(rows)-1 && !selectable(rows[m.cursor]) {
		m.cursor++
	}
	m.clamp()
}

// selectable rows are the ones the cursor may sit on: not gaps, and not art, which the cursor bar would flatten.
func selectable(r row) bool {
	return r.kind != content.Blank && !(r.kind == content.Pre && r.styled != "")
}

// move steps the cursor d rows, then past any rows it may not sit on, in the direction of travel.
// At either end it falls back to the nearest selectable row.
func (m *Model) move(d int) {
	rows := m.rows()
	if len(rows) == 0 {
		return
	}
	step := 1
	if d < 0 {
		step = -1
	}
	c := max(0, min(m.cursor+d, len(rows)-1))
	for i := c; i >= 0 && i < len(rows); i += step {
		if selectable(rows[i]) {
			m.cursor = i
			m.clamp()
			return
		}
	}
	if step < 0 { // nothing to sit on above: go home, which also scrolls the art above back into view
		m.home()
		return
	}
	for i := c; i >= 0; i-- {
		if selectable(rows[i]) {
			m.cursor = i
			break
		}
	}
	m.clamp()
}

func (m *Model) clamp() {
	n := len(m.rows())
	m.cursor = max(0, min(m.cursor, n-1))
	ph := m.paneHeight()
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+ph {
		m.top = m.cursor - ph + 1
	}
	m.top = max(0, min(m.top, n-ph))
}

// View draws the three bands.
func (m Model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	if m.bye {
		// shown inline, not in the alt screen, so it stays on screen after the session ends
		v.AltScreen = false
		v.SetContent(strings.Join(art("sleeping", 0, m.ascii), "\n") + "\n" + kindSt[content.Comment].Render("# idle timeout, bye") + "\n")
		return v
	}
	if m.width < minW || m.height < minH {
		v.SetContent(fmt.Sprintf("terminal too small: need %dx%d", minW, minH))
		return v
	}
	v.SetContent(m.tabBar() + "\n" + m.pane() + "\n" + m.statusBar())
	return v
}

func (m Model) tabBar() string {
	sep := barSep.Render(" │ ")
	cell := func(p content.Page, i int) string {
		if i == m.tab {
			return barDim.Render(fmt.Sprint(p.Key)+" ") + active.Render("["+p.Title+"]")
		}
		return barDim.Render(fmt.Sprint(p.Key)+" ") + bar.Render(p.Title)
	}
	var cells []string
	for i, p := range m.pages {
		cells = append(cells, cell(p, i))
	}
	tabs := strings.Join(cells, sep)
	brand := barGld.Render("▣ agentbox")
	full := brand + sep + tabs + sep + barDim.Render("v0 alpha")
	out := full
	switch {
	case lipgloss.Width(" "+full) <= m.width:
	case lipgloss.Width(" "+tabs) <= m.width && m.width >= 60:
		out = tabs
	default:
		var nums []string
		for i, p := range m.pages {
			if i == m.tab {
				nums = append(nums, active.Render(fmt.Sprintf("[%d %s]", p.Key, p.Title)))
			} else {
				nums = append(nums, barDim.Render(fmt.Sprint(p.Key)))
			}
		}
		out = strings.Join(nums, bar.Render(" "))
	}
	return fill(bar.Render(" ")+out, m.width)
}

// fill pads a bar to the full width with the Ink background, or cuts it.
func fill(s string, w int) string {
	if n := lipgloss.Width(s); n < w {
		return s + bar.Render(strings.Repeat(" ", w-n))
	}
	return ansi.Truncate(s, w, "")
}

func (m Model) pane() string {
	rows := m.rows()
	ph := m.paneHeight()
	tw := m.wrapWidth() + 2
	lines := make([]string, ph)
	cut := m.width
	if m.artSide() {
		cut = tw // long commands and pre blocks must not run under the art
	}
	for i := range lines {
		j := m.top + i
		gutter := "  "
		var s string
		if j < len(rows) {
			if j == m.cursor {
				gutter = st().Foreground(gold).Render("▌") + " "
			}
			s = rows[j].render(tw-2, j == m.cursor)
		}
		lines[i] = ansi.Truncate(gutter+s, cut, "…") // a cut command shows that it goes on; enter copies all of it
	}
	if m.msg != "" {
		s := okSt
		if m.msgErr {
			s = errSt
		}
		lines[ph-1] = ansi.Truncate("  "+s.Render(m.msg), m.width, "")
	}
	if m.artSide() {
		pose, f := m.pose()
		a := art(pose, f, m.ascii)
		for i := range lines {
			pad := max(0, tw-lipgloss.Width(lines[i]))
			if i < len(a) {
				lines[i] += strings.Repeat(" ", pad+2) + a[i]
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) statusBar() string {
	ps := "snib@agentbox:~/" + m.page().Slug + "$ "
	if m.prompting {
		// scroll: show the end of the input, so the caret stays on screen. no hints while typing
		in := m.input
		for len(in) > 0 && 1+ansi.StringWidth(ps)+ansi.StringWidth(string(in))+2 > m.width {
			in = in[1:]
		}
		return fill(bar.Render(" ")+barDim.Render(ps)+bar.Render(string(in))+barGld.Render("▌"), m.width)
	}
	prompt := barDim.Render(ps) + barGld.Render("▌")
	hints := m.shortHelp()
	chip := barDim.Render(status)
	pw, hw, cw := lipgloss.Width(prompt), lipgloss.Width(hints), lipgloss.Width(chip)
	switch {
	case 1+pw+2+cw+2+hw+1 <= m.width:
		mid := m.width - 1 - pw - hw - 1
		left := (mid - cw) / 2
		return bar.Render(" ") + prompt + bar.Render(strings.Repeat(" ", left)) + chip + bar.Render(strings.Repeat(" ", mid-cw-left)) + hints + bar.Render(" ")
	case 1+pw+2+hw+1 <= m.width:
		return bar.Render(" ") + prompt + bar.Render(strings.Repeat(" ", m.width-2-pw-hw)) + hints + bar.Render(" ")
	}
	return fill(bar.Render(" ")+prompt, m.width)
}

// shortHelp is the status-bar hint list. Every cell, the spaces too, has the Ink background, so the bar has no holes.
func (m Model) shortHelp() string {
	var parts []string
	for _, b := range m.keys.ShortHelp() {
		parts = append(parts, barGld.Render(b.Help().Key)+barDim.Render(" "+b.Help().Desc))
	}
	return strings.Join(parts, barSep.Render(" • "))
}

// art returns the ringed pose as lines, from a cache filled once per process.
func art(pose string, frame int, ascii bool) []string {
	c := arts()
	k := fmt.Sprintf("%s/%d/%v", pose, frame, ascii)
	return c[k]
}

var arts = sync.OnceValue(func() map[string][]string {
	c := map[string][]string{}
	for pose := range snib.Poses {
		g, err := snib.Load(pose)
		if err != nil {
			panic(err) // the embedded grids are checked by tests
		}
		for f, rows := range g.Frames {
			lines := func(s string) []string { return strings.Split(strings.TrimRight(s, "\n"), "\n") }
			c[fmt.Sprintf("%s/%d/false", pose, f)] = lines(snib.HalfBlock(g.Palette, rows))
			asc := lines(snib.ASCII(rows))
			for len(asc) < artH {
				asc = append(asc, "")
			}
			c[fmt.Sprintf("%s/%d/true", pose, f)] = asc
		}
	}
	return c
})
