/* azemu mermaid initializer
 * Re-initializes mermaid on every MkDocs Material page navigation
 * and respects the dark/light theme toggle.
 * document$ is the RxJS Subject Material exposes for SPA navigation events.
 */
document$.subscribe(function () {
  var isDark = document.body.getAttribute('data-md-color-scheme') === 'slate';
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
});
