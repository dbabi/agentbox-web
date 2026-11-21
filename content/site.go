package content

// MainCommand is the line the c key copies on every tab.
const MainCommand = "go run github.com/dbabi/agentbox-web@latest"

// Status is the chip in the status line. It states the project state first.
const Status = "early alpha · VMs only · no agent runtime yet"

// Key is one row of the keymap. Both the web help dialog and the TUI help read this list.
type Key struct {
	Keys string
	Help string
	Web  bool // shown on the web
	TUI  bool // shown in the terminal app
}

// Keys is the keymap, in help order.
var Keys = []Key{
	{"1-7", "jump to a tab", true, true},
	{"h l, ← →, [ ]", "previous or next tab", true, true},
	{"tab, shift+tab", "previous or next tab", false, true},
	{"j k, ↓ ↑", "move the line cursor", true, true},
	{"g G", "top or bottom", true, true},
	{"ctrl+d ctrl+u, pgdn pgup", "half page down or up", false, true},
	{"space", "page down", true, true},
	{"enter", "open a link, or copy a $ line", true, true},
	{"c", "copy " + MainCommand, true, true},
	{"/ or :", "focus the prompt: try help, ls, cd security, keys off", true, true},
	{"esc", "leave the prompt, close help", true, true},
	{"?", "this help", true, true},
	{"m", "pause or play motion", true, true},
	{"q, ctrl+c", "quit", false, true},
}
