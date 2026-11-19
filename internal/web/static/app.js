// Marks the page as scripted (hiding buttons that htmx replaces), floats the completion popups, handles the
// Vim-style keys (a cursor over journal entries; Enter logs from the journal bar), the entry editors' keys and the error bar's Dismiss, and collapses project groups on the timeline; the state lives in the browser's local storage.
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
      var cancel = e.target.closest && e.target.closest('.editor .cancel');
      if (cancel) closeEditor(cancel.closest('.editor'));
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
    function panes() {
      return Array.prototype.filter.call(document.querySelectorAll('[data-pane]'), function (p) {
        return p.querySelector('[data-entry]');
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
      if (!restore) return;
      var pane = panes()[restore.pane];
      if (pane) {
        var list = entriesOf(pane);
        moveTo(list[Math.min(restore.entry, list.length - 1)]);
      }
      restore = null;
    });
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
      o: focusBar,
      '[': function () { clickFirst('a[aria-label="Previous week"]'); },
      ']': function () { clickFirst('a[aria-label="Next week"]'); },
      gj: function () { clickFirst('nav a[href="/"]'); },
      gt: function () { clickFirst('nav a[href="/timeline"]'); },
      gp: function () { clickFirst('nav a[href="/projects"]'); },
      O: function () { if (!document.querySelector('a.cancel-new')) clickFirst('a.new-activity'); },
      f: function () {
        var filter = document.getElementById('filter-project');
        if (filter) filter.focus();
      },
      '?': function () {
        var dialog = document.getElementById('keys');
        if (dialog) dialog.showModal();
      },
      Escape: function () {
        var target = document.querySelector('a.cancel-new') || document.querySelector('a.panel-close');
        if (target) target.click();
      }
    };
    document.addEventListener('keydown', function (e) {
      var t = e.target;
      var editor = t && t.closest && t.closest('.editor');
      if (editor && !e.defaultPrevented && !e.isComposing) {
        if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
          e.preventDefault();
          editor.querySelector('.editor-form').requestSubmit();
          return;
        }
        if (e.key === 'Escape') {
          e.preventDefault();
          closeEditor(editor);
          return;
        }
      }
      if (e.ctrlKey || e.altKey || e.metaKey || e.defaultPrevented) return;
      if (e.key === 'Escape' && t && t.id === 'quickadd-text') {
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
      if (document.querySelector('dialog[open]')) return;
      // Enter on the cursor itself follows its activity link; on a link or button it stays theirs.
      if (e.key === 'Enter') {
        var link = t && t.matches && t.matches('[data-entry]') && t.querySelector('a.tag');
        if (link) {
          e.preventDefault();
          link.click();
        }
        return;
      }
      // g and d wait up to a second for the second key of gg, gj, gt, gp and dd.
      var key = (Date.now() - pendingAt < 1000 ? pending : '') + e.key;
      pending = '';
      if (key === 'g' || key === 'd') {
        pending = key;
        pendingAt = Date.now();
        e.preventDefault();
        return;
      }
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
