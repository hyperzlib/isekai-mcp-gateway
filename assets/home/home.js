(() => {
  const guides = [...document.querySelectorAll('.home-guide')];
  const links = [...document.querySelectorAll('[data-client]')];
  function select() {
    const id = location.hash.slice(1);
    const selected = guides.find(g => g.id === id) || guides[0];
    guides.forEach(g => { g.hidden = g !== selected; });
    links.forEach(a => { if (a.dataset.client === selected.id) a.setAttribute('aria-current', 'true'); else a.removeAttribute('aria-current'); });
  }
  select();
  window.addEventListener('hashchange', select);
  let timer;
  document.querySelectorAll('[data-copy]').forEach(button => button.addEventListener('click', async () => {
    const target = document.getElementById(button.dataset.copy);
    const status = document.getElementById('copy-status');
    try {
      await navigator.clipboard.writeText(target.textContent);
      status.textContent = '已复制';
    } catch {
      const range = document.createRange(); range.selectNodeContents(target);
      const selection = window.getSelection(); selection.removeAllRanges(); selection.addRange(range);
      status.textContent = '无法自动复制，已选中文本，请手动复制。';
    }
    clearTimeout(timer); timer = setTimeout(() => { status.textContent = ''; }, 4000);
  }));
})();
