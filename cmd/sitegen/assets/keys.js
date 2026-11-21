// agentbox site keys: the line cursor, the prompt, copy and the motion toggle. No dependencies, no requests.
// Every page works without this file: tabs are links and commands are selectable text.
(function () {
  'use strict';
  var d = document, html = d.documentElement;
  html.classList.remove('no-js');
  html.classList.add('js-caret');
  d.querySelectorAll('.js[hidden]').forEach(function (e) { e.hidden = false; });

  var tabs = [].slice.call(d.querySelectorAll('nav a[data-slug]'));
  var cur = tabs.findIndex(function (a) { return a.hasAttribute('aria-current'); });
  var lines = [].slice.call(d.querySelectorAll('main .ln'));
  var out = d.getElementById('out'), input = d.getElementById('prompt'), field = input.parentNode;
  var help = d.getElementById('help'), keysBtn = d.getElementById('keys');
  var idle = d.querySelector('.idle'), motionBtn = d.getElementById('motion');
  var keysOn = true, timer;
  try { keysOn = localStorage.getItem('keys') !== 'off'; } catch (e) { /* storage blocked: keys stay on */ }

  function say(msg, cls) {
    out.textContent = msg;
    out.className = 'out ' + (cls || '');
    clearTimeout(timer);
    timer = setTimeout(function () { out.textContent = ''; }, 2500);
  }
  function go(i) { if (tabs[i]) location.assign(tabs[i].href); }
  function to(i) {
    i = Math.max(0, Math.min(lines.length - 1, i));
    if (lines[i]) { lines[i].focus(); lines[i].scrollIntoView({ block: 'nearest' }); }
  }
  function at() { var a = d.activeElement; return lines.indexOf(a && a.closest ? a.closest('.ln') : null); }
  function move(n) { var i = at(); to(i < 0 ? (n > 0 ? 0 : lines.length - 1) : i + n); }

  function copy(text, el) {
    function fallback() {
      if (!el) { say('# copy failed. the command is: ' + text, 'err'); return; }
      var r = d.createRange(); r.selectNodeContents(el);
      var s = getSelection(); s.removeAllRanges(); s.addRange(r);
      say('# selected. press ctrl+c or cmd+c to copy');
    }
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text).then(function () { say('# copied', 'ok'); }, fallback);
    } else fallback();
  }
  function act(ln) {
    var a = ln.querySelector('a'), code = ln.querySelector('code');
    if (a) { a.click(); return true; }
    if (code) { copy(code.textContent, code); return true; }
    return false;
  }
  function openHelp() { if (help && !help.open) help.showModal(); }
  function setKeys(on) {
    keysOn = on;
    try { localStorage.setItem('keys', on ? 'on' : 'off'); } catch (e) { /* not saved */ }
    keysBtn.textContent = 'keys: ' + (on ? 'on' : 'off');
    say('# keys ' + (on ? 'on' : 'off'));
  }
  function motion(on) {
    if (!idle) return false;
    if (on === undefined) on = !idle.classList.contains('play');
    idle.classList.toggle('play', on);
    motionBtn.textContent = on ? 'pause' : 'play';
    return true;
  }

  function run(cmd) {
    var arg = cmd.replace(/^(cd|cat)\s+/, '').replace(/^~\//, '').replace(/\/$/, '');
    var i = tabs.findIndex(function (a, n) { return a.dataset.slug === arg || String(n + 1) === arg; });
    if (!cmd) return;
    if (cmd === 'help') openHelp();
    else if (cmd === 'ls') say(tabs.map(function (a) { return a.dataset.slug; }).join('  '));
    else if (cmd === 'clear') { clearTimeout(timer); out.textContent = ''; }
    else if (/^(cd|cd ~|~|cd \.\.)$/.test(cmd)) go(0);
    else if (/^keys (on|off)$/.test(cmd)) setKeys(cmd === 'keys on');
    else if (i >= 0) go(i);
    else say('# command not found: ' + cmd + '. try help', 'err');
  }

  function caret() { field.style.setProperty('--x', input.selectionStart || 0); }
  ['input', 'keyup', 'click', 'focus', 'select'].forEach(function (t) { input.addEventListener(t, caret); });
  input.addEventListener('keydown', function (e) {
    if (e.key === 'Escape') { input.value = ''; caret(); input.blur(); }
    else if (e.key === 'Enter') { e.preventDefault(); var v = input.value.trim(); input.value = ''; caret(); run(v); }
  });
  input.addEventListener('paste', function (e) {
    var s = e.clipboardData ? e.clipboardData.getData('text') : '';
    if (new TextEncoder().encode(s).length > 64) { e.preventDefault(); say('# paste dropped: over 64 bytes', 'err'); }
  });

  d.addEventListener('keydown', function (e) {
    if (e.defaultPrevented || e.ctrlKey || e.altKey || e.metaKey || !keysOn) return;
    var t = e.target;
    if (t.closest && t.closest('input, textarea, select, [contenteditable], dialog')) return;
    var k = e.key, ok = true;
    if (k >= '1' && k <= '9') { if (tabs[k - 1]) go(k - 1); else ok = false; }
    else if (k === 'h' || k === '[' || k === 'ArrowLeft') go(cur - 1);
    else if (k === 'l' || k === ']' || k === 'ArrowRight') go(cur + 1);
    else if (k === 'j' || k === 'ArrowDown') move(1);
    else if (k === 'k' || k === 'ArrowUp') move(-1);
    else if (k === 'g') to(0);
    else if (k === 'G') to(lines.length - 1);
    else if (k === 'Enter') ok = t.classList.contains('ln') && act(t);
    else if (k === 'c') copy(d.body.dataset.cmd);
    else if (k === '/' || k === ':') input.focus();
    else if (k === '?') openHelp();
    else if (k === 'm') ok = motion();
    else ok = false;
    if (ok) e.preventDefault();
  });

  d.querySelectorAll('.copy').forEach(function (b) {
    b.addEventListener('click', function () { var c = b.parentNode.querySelector('code'); copy(c.textContent, c); });
  });
  d.getElementById('helpbtn').addEventListener('click', openHelp);
  keysBtn.addEventListener('click', function () { setKeys(!keysOn); });
  keysBtn.textContent = 'keys: ' + (keysOn ? 'on' : 'off');
  if (motionBtn) {
    motionBtn.addEventListener('click', function () { motion(); });
    motion(!matchMedia('(prefers-reduced-motion: reduce)').matches);
  }
})();
