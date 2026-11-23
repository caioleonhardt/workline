// Marks the page as scripted (hiding buttons that htmx replaces), floats the completion popups, handles the
// Vim-style keys and statusline (a cursor over journal entries; Enter logs from the journal bar), the entry editors' keys and the error bar's Dismiss, and collapses project groups on the timeline; the state lives in the browser's local storage.
// Without this script every group stays expanded and every button shows.
(function () {
  document.documentElement.classList.add('js');
  var KEY = 'workline.collapsed';

  function load() {
    try {
      return JSON.parse(localStorage.getItem(KEY)) || {};
    } catch (e) {
      return {};
    }
  }

  function save(state) {
    try {
      localStorage.setItem(KEY, JSON.stringify(state));
    } catch (e) {}
  }

  function apply(group, collapsed) {
    group.querySelector('.proj-toggle').setAttribute('aria-expanded', String(!collapsed));
    group.querySelectorAll('.act-row').forEach(function (row) {
      row.hidden = collapsed;
    });
  }

  // Document-level handlers. app.js runs again inside every swapped fragment, so
  // they are registered once. The shortcuts only click real links and focus a
  // select, so htmx's own handling of those links applies.
  if (!window.worklineBound) {
    window.worklineBound = true;
    document.addEventListener('click', function (e) {
      var button = e.target.closest && e.target.closest('.error-bar .dismiss');
      if (button) button.closest('.error-bar').remove();
      // Copy Markdown on the report page puts the report's Markdown on the clipboard.
      var copy = e.target.closest && e.target.closest('button.copy-md');
      if (copy && navigator.clipboard) {
        navigator.clipboard.writeText(document.getElementById(copy.dataset.copy).value).then(function () {
          say('Report copied as Markdown');
        }, function () { say('Copy failed', true); });
      }
      var cancel = e.target.closest && e.target.closest('.editor .cancel');
      if (cancel) closeEditor(cancel.closest('.editor'));
      // A click on a journal row, outside its links, controls and editor, opens
      // its editor (selecting text does not).
      var row = e.target.closest && e.target.closest('.feed [data-entry]');
      if (row && !e.target.closest('a, button, input, select, textarea, label, summary, form') && !String(window.getSelection())) {
        var d = row.querySelector('details.edit');
        if (d && !d.open) d.open = true;
      }
    });
    // Entry editors: opening one focuses its text at the end; Cancel or Esc
    // discards the changes and closes it; Ctrl/Cmd+Enter saves.
    function closeEditor(editor) {
      var details = editor.closest('details.edit');
      editor.querySelector('.editor-form').reset();
      editor.querySelectorAll('details.confirm').forEach(function (d) { d.open = false; });
      details.open = false;
      (details.closest('[data-entry]') || details.querySelector('summary')).focus();
    }
    document.addEventListener('toggle', function (e) {
      var d = e.target;
      if (!d.matches || !d.matches('details.edit') || !d.open || d.querySelector('details.confirm[open]')) return;
      var text = d.querySelector('.editor-text');
      text.focus();
      text.setSelectionRange(text.value.length, text.value.length);
    }, true);
    // Completion popups float (position: fixed) beside their dot, above it when
    // there is room, else below, and inside the window horizontally.
    function placePopup(dotWrap) {
      var popup = dotWrap.querySelector('.popup');
      if (!popup) return;
      dotWrap.classList.add('open');
      var r = dotWrap.querySelector('.dot').getBoundingClientRect();
      var w = popup.offsetWidth, h = popup.offsetHeight, gap = 4, margin = 8;
      var left = Math.min(Math.max(margin, r.left + r.width / 2 - w / 2), window.innerWidth - w - margin);
      var top = r.top - h - gap;
      if (top < margin) top = Math.min(r.bottom + gap, window.innerHeight - h - margin);
      popup.style.left = left + 'px';
      popup.style.top = top + 'px';
    }
    function closePopup(dotWrap) {
      if (!dotWrap.matches(':hover') && !dotWrap.matches(':focus-within')) dotWrap.classList.remove('open');
    }
    function dotOf(e) {
      return e.target.closest && e.target.closest('.done-dot');
    }
    document.addEventListener('mouseover', function (e) {
      var d = dotOf(e);
      if (d && !d.classList.contains('open')) placePopup(d);
    });
    document.addEventListener('focusin', function (e) {
      var d = dotOf(e);
      if (d) placePopup(d);
    });
    document.addEventListener('mouseout', function (e) {
      var d = dotOf(e);
      if (d && !d.contains(e.relatedTarget)) {
        d.classList.remove('open');
        if (d.matches(':focus-within')) placePopup(d);
      }
    });
    document.addEventListener('focusout', function (e) {
      var d = dotOf(e);
      if (d) setTimeout(function () { closePopup(d); }, 0);
    });
    // A fixed popup would drift from its dot on scroll: move it along.
    document.addEventListener('scroll', function () {
      document.querySelectorAll('.done-dot.open').forEach(placePopup);
    }, true);
    // Normal-mode keys (SPEC §10 item 99). Entry lists are panes ([data-pane])
    // and the focused entry ([data-entry]) is the cursor; fields keep their keys.
    function focusBar() {
      var field = document.getElementById('quickadd-text');
      if (!field || !document.querySelector('#quickadd-target .target')) return false;
      field.focus();
      return true;
    }
    // Panes in screen order, left to right then top to bottom: on the timeline the
    // panel comes first in the markup but is drawn to the right of the feed.
    function panes() {
      return Array.prototype.filter.call(document.querySelectorAll('[data-pane]'), function (p) {
        return p.querySelector('[data-entry]');
      }).sort(function (a, b) {
        var ra = a.getBoundingClientRect(), rb = b.getBoundingClientRect();
        return ra.left - rb.left || ra.top - rb.top;
      });
    }
    function entriesOf(pane) {
      return Array.prototype.slice.call(pane.querySelectorAll('[data-entry]'));
    }
    function cursor() {
      var a = document.activeElement;
      return a && a.closest ? a.closest('[data-entry]') : null;
    }
    function moveTo(entry) {
      if (!entry) return;
      entry.focus();
      entry.scrollIntoView({ block: 'nearest' });
    }
    // step moves within the cursor's pane; from no cursor it starts at the first entry.
    function step(delta) {
      var cur = cursor(), ps = panes();
      if (!cur) return ps.length && moveTo(entriesOf(ps[0])[0]);
      var list = entriesOf(cur.closest('[data-pane]'));
      moveTo(list[Math.max(0, Math.min(list.length - 1, list.indexOf(cur) + delta))]);
    }
    function edge(last) {
      var cur = cursor(), ps = panes();
      var pane = cur ? cur.closest('[data-pane]') : ps[0];
      if (!pane) return;
      var list = entriesOf(pane);
      moveTo(list[last ? list.length - 1 : 0]);
    }
    function switchPane(delta) {
      var cur = cursor(), ps = panes();
      var i = cur ? ps.indexOf(cur.closest('[data-pane]')) + delta : 0;
      if (ps[i]) moveTo(entriesOf(ps[i])[0]);
    }
    // edit opens the cursor's editor; with confirm it goes to the Delete… step.
    function edit(confirm) {
      var cur = cursor(), d = cur && cur.querySelector('details.edit');
      if (!d) return;
      if (confirm) d.querySelector('details.confirm').open = true;
      d.open = true;
      if (confirm) d.querySelector('details.confirm button').focus();
    }
    function clickFirst(selector) {
      var el = document.querySelector(selector);
      if (el) el.click();
    }
    // A form sent from an entry (toggle, save, delete) swaps the page; the cursor
    // comes back at the same place in the same pane.
    var restore = null;
    document.addEventListener('submit', function (e) {
      var entry = e.target.closest && e.target.closest('[data-entry]');
      if (!entry) return;
      var pane = entry.closest('[data-pane]');
      restore = { pane: panes().indexOf(pane), entry: entriesOf(pane).indexOf(entry) };
    }, true);
    document.addEventListener('htmx:afterSettle', function () {
      updateStatus();
      searchPageStatus();
      if (pattern && hits) fetchHits(mark);
      if (!restore) return;
      var pane = panes()[restore.pane];
      if (pane) {
        var list = entriesOf(pane);
        moveTo(list[Math.min(restore.entry, list.length - 1)]);
      }
      restore = null;
    });
    // The statusline (SPEC §10 item 101): mode, messages, pending keys and the
    // cursor's place, as in Neovim. It sits outside #dash, so swaps keep it.
    function say(text, error) {
      var msg = document.getElementById('status-msg');
      if (!msg) return;
      msg.textContent = text;
      msg.classList.toggle('error', !!error);
    }
    var cmdTimer = null;
    function showcmd(text) {
      var cmd = document.getElementById('status-cmd');
      if (cmd) cmd.textContent = text;
      clearTimeout(cmdTimer);
      if (text) cmdTimer = setTimeout(function () { showcmd(''); }, 1000);
    }
    function updateStatus() {
      var mode = document.getElementById('status-mode');
      if (!mode) return;
      var a = document.activeElement, name = 'NORMAL';
      if (a && a.id === 'vsearch-text') {
        name = 'COMMAND';
      } else if (a && (a.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(a.tagName) || a.closest('.editor'))) {
        name = 'INSERT';
      }
      mode.textContent = name;
      mode.className = 'mode ' + name.toLowerCase();
      var cur = cursor(), pos = '';
      if (cur) {
        var pane = cur.closest('[data-pane]'), list = entriesOf(pane);
        pos = pane.dataset.pane + ' ' + (list.indexOf(cur) + 1) + '/' + list.length;
      }
      document.getElementById('status-pos').textContent = pos;
    }
    document.addEventListener('focusin', updateStatus);
    document.addEventListener('focusout', function () { setTimeout(updateStatus, 0); });
    // Search (SPEC §10 item 103). The server parses the query; Enter marks the
    // entries on the page whose ids it returns and jumps between them, Ctrl+Enter
    // opens the search page. The marks are fetched again after every swap.
    var pattern = '', hits = null, total = 0, searchFrom = null;
    // Search history, as in Vim's cmdline: Ctrl+P or Up recalls older searches,
    // Ctrl+N or Down newer ones. It is kept in local storage (newest last, 50 at
    // most) so it survives Ctrl+Enter's page load.
    var HISTORY = 'workline.searches', histPos = 0, draft = '';
    function history() {
      try {
        var list = JSON.parse(localStorage.getItem(HISTORY));
        return Array.isArray(list) ? list : [];
      } catch (e) {
        return [];
      }
    }
    function remember(q) {
      var list = history().filter(function (old) { return old !== q; });
      list.push(q);
      try {
        localStorage.setItem(HISTORY, JSON.stringify(list.slice(-50)));
      } catch (e) {
        // Without storage the history lasts only as long as the page.
      }
    }
    function recall(input, delta) {
      var list = history();
      if (histPos === list.length) draft = input.value;
      histPos = Math.max(0, Math.min(list.length, histPos + delta));
      input.value = histPos === list.length ? draft : list[histPos];
    }
    function fetchHits(then) {
      fetch('/search/ids?' + new URLSearchParams({ q: pattern })).then(function (res) {
        return res.json().then(function (body) {
          if (!res.ok) {
            hits = null;
            mark();
            return say(body.error, true);
          }
          hits = {};
          total = body.ids.length;
          body.ids.forEach(function (id) { hits[id] = true; });
          then();
        });
      }).catch(function () { say('Search failed', true); });
    }
    // mark highlights the hits on the page and returns them in page order.
    function mark() {
      return Array.prototype.filter.call(document.querySelectorAll('[data-entry]'), function (entry) {
        var hit = !!hits && hits[entry.dataset.id] === true;
        entry.classList.toggle('match', hit);
        return hit;
      });
    }
    function search(delta) {
      if (!pattern) return say('E35: No previous regular expression', true);
      if (!hits) return;
      var all = Array.prototype.slice.call(document.querySelectorAll('[data-entry]'));
      var found = mark();
      if (!found.length && total) return say('E486: Pattern not found on this page: ' + pattern + ' (' + total + ' in the journal, Ctrl+Enter lists them)', true);
      if (!found.length) return say('E486: Pattern not found: ' + pattern, true);
      var at = all.indexOf(cursor()), next = null;
      found.forEach(function (entry) {
        var i = all.indexOf(entry);
        if (delta > 0 ? !next && i > at : i < at || at === -1) next = entry;
      });
      var target = next || found[delta > 0 ? 0 : found.length - 1];
      moveTo(target);
      if (next) {
        say('/' + pattern + '  [' + (found.indexOf(target) + 1) + '/' + found.length + ']' + (total > found.length ? ' · ' + total + ' in the journal' : ''));
      } else {
        say(delta > 0 ? 'search hit BOTTOM, continuing at TOP' : 'search hit TOP, continuing at BOTTOM', true);
      }
    }
    function clearSearch() {
      pattern = '';
      hits = null;
      mark();
      say('');
    }
    function openSearch() {
      var form = document.querySelector('.vsearch');
      if (!form) return;
      searchFrom = cursor();
      completion = null;
      histPos = history().length;
      say('');
      form.hidden = false;
      form.querySelector('input').value = '';
      form.querySelector('input').focus();
    }
    // closeSearch leaves the search line: how is 'cancel', 'mark' (Enter) or
    // 'filter' (Ctrl+Enter). An empty line repeats the last pattern, as in Vim.
    function closeSearch(how) {
      var input = document.getElementById('vsearch-text');
      var typed = input.value.trim();
      input.closest('.vsearch').hidden = true;
      if (searchFrom && searchFrom.isConnected) searchFrom.focus();
      else input.blur();
      if (how === 'cancel') return say('');
      if (!typed && !pattern) return say('E35: No previous regular expression', true);
      if (typed) remember(typed);
      if (how === 'filter') {
        var from = location.pathname + location.search;
        if (location.pathname === '/search') from = new URLSearchParams(location.search).get('from') || '/';
        location.assign('/search?' + new URLSearchParams({ q: typed || pattern, from: from }));
        return;
      }
      if (typed) pattern = typed;
      fetchHits(function () { search(1); });
    }
    // The search page tells the statusline what it shows.
    function searchPageStatus() {
      var dash = document.querySelector('#dash[data-search]');
      if (!dash) return;
      if (!pattern) pattern = dash.dataset.search;
      var refused = dash.querySelector('.error-bar[role="alert"]');
      if (refused) return say(refused.textContent, true);
      say('/' + dash.dataset.search + '  [filter] ' + dash.dataset.count + (dash.dataset.count === '1' ? ' result' : ' results'));
    }
    // app.js loads inside #dash, before the statusline's markup.
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', searchPageStatus);
    else searchPageStatus();
    // Tab completes the search line's last token from /search/words (fetched
    // once), cycling through the candidates like Vim's wildmenu.
    var words = null, completion = null;
    function tokenStart(s) {
      var start = 0, quoted = false;
      for (var i = 0; i < s.length; i++) {
        if (s[i] === '"') quoted = !quoted;
        else if (/\s/.test(s[i]) && !quoted) start = i + 1;
      }
      return start;
    }
    function candidates(token) {
      var neg = token.charAt(0) === '!' ? '!' : '', body = token.slice(neg.length);
      var colon = body.indexOf(':');
      if (colon === -1) {
        return words.keys.map(function (k) { return k.key + ':'; })
          .filter(function (c) { return c.indexOf(body) === 0; })
          .map(function (c) { return neg + c; });
      }
      var key = body.slice(0, colon), typed = body.slice(colon + 1).replace(/"/g, '').toLowerCase();
      var spec = words.keys.filter(function (k) { return k.key === key; })[0];
      if (!spec) return [];
      var values = spec.values.length ? spec.values : key === 'proj' ? words.projects : words.activities;
      return values.filter(function (v) { return v.toLowerCase().indexOf(typed) === 0; }).map(function (v) {
        return neg + key + ':' + (/\s/.test(v) ? '"' + v + '"' : v);
      });
    }
    function wildmenu(list, current) {
      var msg = document.getElementById('status-msg');
      msg.textContent = '';
      msg.classList.remove('error');
      list.forEach(function (c, i) {
        var span = document.createElement('span');
        span.className = i === current ? 'wild current' : 'wild';
        span.textContent = c;
        msg.appendChild(span);
      });
    }
    function complete(input, delta) {
      if (!words) {
        fetch('/search/words').then(function (res) { return res.json(); }).then(function (w) {
          words = w;
          complete(input, delta);
        }).catch(function () { say('Search failed', true); });
        return;
      }
      if (!completion) {
        var head = input.value.slice(0, tokenStart(input.value));
        var list = candidates(input.value.slice(head.length));
        if (!list.length) return;
        completion = { head: head, list: list, i: delta > 0 ? -1 : list.length };
      }
      var c = completion, n = c.list.length;
      c.i = (c.i + delta + n) % n;
      input.value = c.head + c.list[c.i];
      wildmenu(c.list, c.i);
      // A single candidate is taken, so the next Tab completes what follows it (p: → its values).
      if (n === 1) completion = null;
    }
    var pending = '', pendingAt = 0;
    var keys = {
      j: function () { step(1); },
      k: function () { step(-1); },
      gg: function () { edge(false); },
      G: function () { edge(true); },
      h: function () { switchPane(-1); },
      l: function () { switchPane(1); },
      i: function () { edit(false); },
      dd: function () { edit(true); },
      x: function () {
        var cur = cursor(), form = cur && cur.querySelector('.sub-toggle');
        if (form) form.requestSubmit();
      },
      s: function () {
        var cur = cursor(), form = cur && cur.querySelector('.star-form');
        if (form) form.requestSubmit();
      },
      o: focusBar,
      '[': function () { clickFirst('a[aria-label="Previous week"]'); },
      ']': function () { clickFirst('a[aria-label="Next week"]'); },
      gj: function () { clickFirst('nav a[href="/"]'); },
      gt: function () { clickFirst('nav a[href="/timeline"]'); },
      gr: function () { clickFirst('nav a[href="/report"]'); },
      gp: function () { clickFirst('nav a[href="/projects"]'); },
      O: function () { if (!document.querySelector('a.cancel-new')) clickFirst('a.new-activity'); },
      f: function () {
        var filter = document.getElementById('filter-project');
        if (filter) filter.focus();
      },
      '/': openSearch,
      n: function () { search(1); },
      N: function () { search(-1); },
      '?': function () {
        var dialog = document.getElementById('keys');
        if (dialog) dialog.showModal();
      },
      Escape: function () {
        if (pattern) return clearSearch();
        var target = document.querySelector('a.cancel-new') || document.querySelector('a.panel-close') || document.querySelector('a.clear-search');
        if (target) target.click();
      }
    };
    document.addEventListener('keydown', function (e) {
      var t = e.target;
      // Ctrl+[ is Esc, as in Vim.
      var esc = e.key === 'Escape' || (e.key === '[' && e.ctrlKey && !e.altKey && !e.metaKey);
      var editor = t && t.closest && t.closest('.editor');
      if (editor && !e.defaultPrevented && !e.isComposing) {
        if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
          e.preventDefault();
          editor.querySelector('.editor-form').requestSubmit();
          return;
        }
        if (esc) {
          e.preventDefault();
          closeEditor(editor);
        }
      }
      if (editor) return; // the editor is Insert mode, its buttons included
      // The search line: Enter marks, Ctrl+Enter filters, Esc goes back to the cursor, Tab completes.
      if (t && t.id === 'vsearch-text' && !e.isComposing) {
        if (e.key === 'Tab') {
          e.preventDefault();
          return complete(t, e.shiftKey ? -1 : 1);
        }
        if (!/^(Shift|Control|Alt|Meta)$/.test(e.key)) completion = null;
        var older = e.key === 'ArrowUp' || (e.ctrlKey && e.key === 'p');
        if (older || e.key === 'ArrowDown' || (e.ctrlKey && e.key === 'n')) {
          e.preventDefault();
          return recall(t, older ? -1 : 1);
        }
        if (e.key === 'Enter' || esc) {
          e.preventDefault();
          closeSearch(esc ? 'cancel' : e.ctrlKey || e.metaKey ? 'filter' : 'mark');
        }
        return;
      }
      if ((e.ctrlKey && !esc) || e.altKey || e.metaKey || e.defaultPrevented) return;
      if (esc && t && t.id === 'quickadd-text') {
        e.preventDefault();
        t.blur();
        return;
      }
      // In the bar, Enter logs the entry and Shift+Enter starts a new line.
      if (e.key === 'Enter' && !e.shiftKey && !e.isComposing && t && t.id === 'quickadd-text') {
        e.preventDefault();
        t.form.requestSubmit();
        return;
      }
      if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT|SUMMARY)$/.test(t.tagName))) return;
      var dialog = document.querySelector('dialog[open]');
      if (dialog) {
        if (esc && e.key !== 'Escape') dialog.close(); // the browser closes it on Esc itself
        return;
      }
      // Enter on the cursor itself follows its activity link; on a link or button it stays theirs.
      if (e.key === 'Enter') {
        var link = t && t.matches && t.matches('[data-entry]') && t.querySelector('a.tag');
        if (link) {
          e.preventDefault();
          link.click();
        }
        return;
      }
      // g and d wait up to a second for the second key of gg, gj, gt, gr, gp and dd.
      var key = (Date.now() - pendingAt < 1000 ? pending : '') + (esc ? 'Escape' : e.key);
      pending = '';
      if (key === 'g' || key === 'd') {
        pending = key;
        pendingAt = Date.now();
        showcmd(key);
        e.preventDefault();
        return;
      }
      showcmd('');
      if (keys[key]) {
        e.preventDefault();
        keys[key]();
      }
    });
  }

  var state = load();
  document.querySelectorAll('.grp[data-project]').forEach(function (group) {
    var id = group.dataset.project;
    var toggle = group.querySelector('.proj-toggle');
    apply(group, !!state[id]);
    toggle.addEventListener('click', function () {
      var collapse = toggle.getAttribute('aria-expanded') === 'true';
      apply(group, collapse);
      state = load();
      if (collapse) {
        state[id] = true;
      } else {
        delete state[id];
      }
      save(state);
    });
  });
})();
