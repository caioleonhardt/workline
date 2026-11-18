// Marks the page as scripted (hiding buttons that htmx replaces) and collapses
// project groups on the timeline; the state lives in the browser's local storage.
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
