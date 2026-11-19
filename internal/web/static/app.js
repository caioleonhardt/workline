// Marks the page as scripted (hiding buttons that htmx replaces), floats the completion popups, handles the
// keyboard shortcuts (including Enter to log from the journal bar), the entry editors' keys and the error bar's Dismiss, and collapses project groups on the timeline; the state lives in the browser's local storage.
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
      details.querySelector('summary').focus();
    }
    document.addEventListener('toggle', function (e) {
      var d = e.target;
      if (!d.matches || !d.matches('details.edit') || !d.open) return;
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
    // Single keys focus the journal bar with a kind chosen (j keeps the current one).
    var barKinds = { t: 'task', d: 'decision', u: 'update', b: 'blocker', m: 'meeting' };
    function focusBar(kind) {
      var field = document.getElementById('quickadd-text');
      if (!field || !document.querySelector('#quickadd-target .target')) return false;
      var select = document.getElementById('quickadd-kind');
      if (kind && select) select.value = kind;
      field.focus();
      return true;
    }
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
      var cancel = document.querySelector('a.cancel-new');
      var target = null;
      if (e.key === 'n' && !cancel) {
        target = document.querySelector('a.new-activity');
      } else if (e.key === 'Escape') {
        target = cancel || document.querySelector('a.panel-close');
      } else if (e.key === 'j' || barKinds[e.key]) {
        if (focusBar(barKinds[e.key])) e.preventDefault();
        return;
      } else if (e.key === '/') {
        // The timeline's project filter; elsewhere the bar, rather than the browser's quick find.
        var filter = document.getElementById('filter-project');
        if (filter) {
          e.preventDefault();
          filter.focus();
        } else if (focusBar()) {
          e.preventDefault();
        }
        return;
      }
      if (target) {
        e.preventDefault();
        target.click();
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
