/* azemu mermaid initializer
 * Re-initializes mermaid on every MkDocs Material page navigation
 * and re-renders on palette (dark/light) toggle.
 *
 * document$ is the RxJS Subject Material exposes for SPA navigation events.
 * MutationObserver watches body[data-md-color-scheme] for palette changes,
 * since palette toggling does not emit document$.
 */

function initMermaid() {
  var isDark = document.body.getAttribute('data-md-color-scheme') === 'slate';

  // On palette toggle: restore original diagram source so mermaid can re-render.
  // mermaid replaces .mermaid textContent with an SVG on first run; we must
  // save the source before that happens (stored in data-mermaid-source).
  // textContent is used (not innerHTML) because diagram source is plain text.
  document.querySelectorAll('.mermaid[data-mermaid-source]').forEach(function (el) {
    el.textContent = el.getAttribute('data-mermaid-source');
    el.removeAttribute('data-processed');
  });

  // Save original source for any diagram not yet processed (first run or new page).
  document.querySelectorAll('.mermaid:not([data-mermaid-source])').forEach(function (el) {
    el.setAttribute('data-mermaid-source', el.textContent.trim());
  });

  mermaid.initialize({
    startOnLoad: false,
    theme: isDark ? 'dark' : 'default',
    themeVariables: isDark
      ? {
          primaryColor: '#238636',
          primaryTextColor: '#e6edf3',
          primaryBorderColor: '#30363d',
          lineColor: '#8b949e',
          background: '#0d1117',
          nodeBorder: '#30363d',
          clusterBkg: '#161b22',
          titleColor: '#e6edf3',
          edgeLabelBackground: '#161b22',
          nodeTextColor: '#e6edf3',
        }
      : {},
  });

  mermaid.run();
}

// Re-render on SPA navigation (new page content, fresh .mermaid elements).
document$.subscribe(initMermaid);

// Re-render on palette toggle (dark ↔ light).
// Material sets data-md-color-scheme on <body> without emitting document$.
new MutationObserver(function (mutations) {
  mutations.forEach(function (mutation) {
    if (mutation.attributeName === 'data-md-color-scheme') {
      initMermaid();
    }
  });
}).observe(document.body, {
  attributes: true,
  attributeFilter: ['data-md-color-scheme'],
});
