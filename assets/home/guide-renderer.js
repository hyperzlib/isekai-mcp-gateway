(function (root, factory) {
  if (typeof module === 'object' && module.exports) module.exports = factory;
  else root.HomeGuides = factory(root.markdownit, root.markdownitContainer, root.DOMPurify);
})(globalThis, function (markdownit, container, purifier) {
  'use strict';
  const indexPath = '/assets/home/guides/index.json';
  function replaceMacros(text, endpoint) {
    const values = { mcp_url: endpoint, mcp_url_json: JSON.stringify(endpoint),
      mcp_url_shell: "'" + endpoint.replace(/'/g, "'\"'\"'") + "'",
      mcp_url_powershell: "'" + endpoint.replace(/'/g, "''") + "'" };
    return text.replace(/\{\{([^{}]*)\}\}/g, (_, name) => {
      name = name.trim();
      if (!Object.hasOwn(values, name)) throw new Error('未知教程宏：' + name);
      return values[name];
    });
  }
  function validateIndex(raw, origin) {
    if (!Array.isArray(raw) || !raw.length) throw new Error('教程索引必须是非空数组。');
    const seen = new Set(), base = new URL(indexPath, origin);
    return raw.map(item => {
      if (!item || typeof item.id !== 'string' || !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(item.id) || seen.has(item.id) ||
          typeof item.name !== 'string' || !item.name.trim() || typeof item.icon !== 'string' || !item.icon ||
          typeof item.guide !== 'string' || !/^[a-zA-Z0-9_-]+(?:\/[a-zA-Z0-9_-]+)*\.md$/.test(item.guide)) throw new Error('教程索引字段、ID 或路径无效。');
      seen.add(item.id);
      const icon = new URL(item.icon, base);
      if (icon.username || icon.password || (icon.protocol !== 'https:' && !(icon.origin === base.origin && icon.protocol === base.protocol))) throw new Error('图标必须使用同源或 HTTPS 地址。');
      return { id: item.id, name: item.name.trim(), icon: icon.href, guide: new URL(item.guide, base).href };
    });
  }
  function render(source, endpoint, guideURL) {
    const md = markdownit({ html: false, linkify: false, breaks: false });
    md.use(container, 'details', {
      validate: info => /^details\s+\S/.test(info.trim()),
      render: (tokens, idx) => tokens[idx].nesting === 1
        ? '<details class="home-snippet-toggle"><summary>' + md.utils.escapeHtml(tokens[idx].info.trim().slice(7).trim()) + '</summary>\n' : '</details>\n'
    });
    const fence = md.renderer.rules.fence;
    md.renderer.rules.fence = (tokens, idx, options, env, self) => {
      const token = tokens[idx], original = token.info;
      const match = original.match(/(?:^|\s)title="([^"\r\n]*)"\s*$/);
      token.info = original.split(/\s/)[0];
      const code = fence(tokens, idx, options, env, self);
      token.info = original;
      return '<div class="home-snippet" data-code-title="' + md.utils.escapeHtml(match ? match[1] : original.split(/\s/)[0] || '代码') + '">' + code + '</div>\n';
    };
    let depth = 0;
    function visit(tokens) {
      for (const token of tokens) {
        if (token.type === 'container_details_open' && ++depth > 1) throw new Error('教程暂不支持嵌套折叠块。');
        if (token.type === 'container_details_close') depth--;
        if (['text', 'code_inline', 'fence', 'code_block'].includes(token.type)) token.content = replaceMacros(token.content, endpoint);
        if (token.type === 'link_open' || token.type === 'image') {
          const attr = token.type === 'image' ? 'src' : 'href';
          const url = new URL(token.attrGet(attr), guideURL);
          if (!['https:', 'http:', ...(attr === 'href' ? ['mailto:'] : [])].includes(url.protocol) || url.username || url.password) throw new Error('教程包含不支持的链接地址。');
          token.attrSet(attr, url.href);
        }
        if (token.children) visit(token.children);
      }
    }
    const tokens = md.parse(source, {}); visit(tokens);
    return purifier.sanitize(md.renderer.render(tokens, md.options, {}), {
      USE_PROFILES: { html: true }, FORBID_TAGS: ['style', 'form', 'input', 'button', 'iframe'], FORBID_ATTR: ['style', 'id', 'name']
    });
  }
  return { indexPath, replaceMacros, validateIndex, render };
});
