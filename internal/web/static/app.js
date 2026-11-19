// Marks the page as scripted (hiding buttons that htmx replaces), handles the
// keyboard shortcuts and the error bar's Dismiss, and collapses project groups on the timeline; the state lives in the browser's local storage.
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
    });
    document.addEventListener('keydown', function (e) {
      var t = e.target;
      if (e.ctrlKey || e.altKey || e.metaKey || e.defaultPrevented) return;
      if (e.key === 'Escape' && t && t.id === 'quickadd-text') {
        e.preventDefault();
        t.blur();
        return;
      }
      if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT|SUMMARY)$/.test(t.tagName))) return;
      var cancel = document.querySelector('a.cancel-new');
      var target = null;
      if (e.key === 'n' && !cancel) {
        target = document.querySelector('a.new-activity');
      } else if (e.key === 'Escape') {
        target = cancel || document.querySelector('a.panel-close');
      } else if (e.key === 'j') {
        var field = document.getElementById('quickadd-text');
        if (field && document.querySelector('#quickadd-target .target')) {
          e.preventDefault();
          field.focus();
        }
        return;
      } else if (e.key === '/') {
        var filter = document.getElementById('filter-project');
        if (filter) {
          e.preventDefault();
          filter.focus();
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
