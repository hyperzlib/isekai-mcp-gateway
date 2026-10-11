(() => {
  'use strict';
  const section = document.querySelector('.home-connect');
  const list = document.getElementById('client-list');
  const article = document.getElementById('client-guide');
  const scroll = article.querySelector('.home-guide-scroll');
  const content = article.querySelector('.home-guide-content');
  const status = document.getElementById('guide-status');
  const retry = document.getElementById('guide-retry');
  const state = document.querySelector('.home-guide-state');
  const spinner = document.querySelector('.home-guide-spinner');
  const cache = new Map();
  let clients = [], request = 0, retryAction, timer;
  const element = (tag, text, className) => {
    const node = document.createElement(tag);
    if (text !== undefined) node.textContent = text;
    if (className) node.className = className;
    return node;
  };
  // Observe unconstrained content, not the viewport whose height we animate.
  // This also detects image loads and details expansion beyond the 80vh cap.
  function animateCardHeight(card, viewport, inner) {
    let frame = 0;
    const number = value => parseFloat(value) || 0;
    function schedule() {
      if (frame) return;
      frame = requestAnimationFrame(() => {
        frame = 0;
        if (card.hidden) return;
        const outerStyle = getComputedStyle(card), scrollStyle = getComputedStyle(viewport);
        const chrome = number(outerStyle.paddingTop) + number(outerStyle.paddingBottom) +
          number(outerStyle.borderTopWidth) + number(outerStyle.borderBottomWidth) +
          number(scrollStyle.paddingTop) + number(scrollStyle.paddingBottom);
        const height = Math.min(inner.getBoundingClientRect().height + chrome, number(outerStyle.maxHeight));
        const target = height + 'px';
        if (card.style.height !== target) card.style.height = target;
      });
    }
    const observer = new ResizeObserver(schedule);
    observer.observe(inner);
    window.addEventListener('resize', schedule);
    schedule();
  }
  if (window.ResizeObserver) {
    animateCardHeight(article, scroll, content);
    const clientCard = document.querySelector('.home-clients');
    animateCardHeight(clientCard, clientCard.querySelector('.home-clients-scroll'), clientCard.querySelector('.home-clients-content'));
  }
  function icon(client, large = false) {
    const image = element('img', undefined, 'home-client-logo' + (large ? ' home-client-logo--lg' : ''));
    image.alt = ''; image.src = client.icon;
    image.addEventListener('error', () => { image.src = '/assets/icons/openai.svg'; }, { once: true });
    return image;
  }
  function loading(text) {
    status.textContent = text; retry.hidden = true; spinner.hidden = false; state.hidden = false;
    content.replaceChildren(state); scroll.scrollTop = 0; article.hidden = false;
    article.removeAttribute('aria-labelledby');
    article.setAttribute('aria-busy', 'true');
  }
  function failed(error, action) {
    status.textContent = '无法加载教程：' + error.message;
    spinner.hidden = true;
    retryAction = action; retry.hidden = false; article.removeAttribute('aria-busy');
  }
  async function fetchContent(url, json = false) {
    const response = await fetch(url, { cache: 'no-cache', credentials: 'same-origin' });
    if (!response.ok) throw new Error('HTTP ' + response.status);
    return json ? response.json() : response.text();
  }
  function decorateCodes() {
    article.querySelectorAll('pre').forEach(pre => {
      let wrapper = pre.parentElement;
      if (!wrapper.classList.contains('home-snippet')) {
        wrapper = element('div', undefined, 'home-snippet'); pre.before(wrapper); wrapper.append(pre);
      }
      const bar = element('div'); bar.append(element('span', wrapper.dataset.codeTitle || '代码'));
      wrapper.removeAttribute('data-code-title');
      const button = element('button', '复制'); button.type = 'button'; button.dataset.copyCode = '';
      bar.append(button); wrapper.prepend(bar);
    });
    article.querySelectorAll('a[href]').forEach(link => {
      if (new URL(link.href).origin !== location.origin) { link.target = '_blank'; link.rel = 'noopener noreferrer'; }
    });
  }
  async function select() {
    if (!clients.length) return;
    const current = ++request;
    const client = clients.find(c => c.id === location.hash.slice(1)) || clients[0];
    list.querySelectorAll('a').forEach(link => {
      if (link.dataset.client === client.id) link.setAttribute('aria-current', 'true');
      else link.removeAttribute('aria-current');
    });
    loading('正在加载 ' + client.name + ' 教程…');
    try {
      let html = cache.get(client.id);
      if (html === undefined) {
        const source = await fetchContent(client.guide);
        html = HomeGuides.render(source, section.dataset.mcpEndpoint, client.guide);
        cache.set(client.id, html);
      }
      if (current !== request) return;
      const head = element('div', undefined, 'home-guide-head'), title = element('div');
      const heading = element('h2', client.name); heading.id = 'guide-title';
      article.setAttribute('aria-labelledby', heading.id);
      title.append(element('span', '添加 MCP 服务器', 'home-label'), heading);
      head.append(icon(client, true), title);
      const body = element('div', undefined, 'home-guide-body'); body.innerHTML = html;
      state.hidden = true; content.replaceChildren(state, head, body); scroll.scrollTop = 0; decorateCodes();
      article.hidden = false; article.removeAttribute('aria-busy'); status.textContent = ''; spinner.hidden = true;
    } catch (error) { if (current === request) failed(error, select); }
  }
  async function loadIndex() {
    loading('正在加载客户端列表…');
    try {
      if (!window.HomeGuides) throw new Error('教程渲染组件未加载，请刷新页面。');
      clients = HomeGuides.validateIndex(await fetchContent(HomeGuides.indexPath, true), location.origin);
      list.replaceChildren(...clients.map(client => {
        const link = element('a'); link.href = '#' + client.id; link.dataset.client = client.id;
        link.append(icon(client), document.createTextNode(client.name));
        link.addEventListener('click', () => { if (location.hash === '#' + client.id) select(); });
        return link;
      }));
      await select();
    } catch (error) { failed(error, loadIndex); }
  }
  retry.addEventListener('click', () => retryAction());
  window.addEventListener('hashchange', select);
  document.addEventListener('click', async event => {
    const button = event.target.closest('button[data-copy], button[data-copy-code]');
    if (!button) return;
    const target = button.hasAttribute('data-copy-code') ? button.closest('.home-snippet').querySelector('pre code, pre') : document.getElementById(button.dataset.copy);
    if (!target) return;
    const copyStatus = document.getElementById('copy-status');
    try { await navigator.clipboard.writeText(target.textContent); copyStatus.textContent = '已复制'; }
    catch {
      const range = document.createRange(); range.selectNodeContents(target);
      const selection = window.getSelection(); selection.removeAllRanges(); selection.addRange(range);
      copyStatus.textContent = '无法自动复制，已选中文本，请手动复制。';
    }
    clearTimeout(timer); timer = setTimeout(() => { copyStatus.textContent = ''; }, 4000);
  });
  loadIndex();
})();
