// Run against: go run ./internal/home/browser-preview
// HOME_TEST_URL=http://127.0.0.1:<port> node tests/home-browser.cjs
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require('playwright');
const base = process.env.HOME_TEST_URL;
if (!base) throw new Error('Set HOME_TEST_URL to the browser-preview URL');
const index = JSON.parse(fs.readFileSync('assets/home/guides/index.json', 'utf8'));
const endpoint = process.env.HOME_TEST_ENDPOINT || 'https://wiki.example/mcp?test=1&mode=read';
const expectedCounts = { 'cherry-studio': [4, 1], opencode: [3, 3], eigent: [4, 1], chatgpt: [4, 1], claude: [4, 1], codex: [2, 1], 'claude-code': [3, 1], cursor: [3, 1], vscode: [4, 1] };

(async () => {
  const browser = await chromium.launch({ channel: process.env.HOME_TEST_BROWSER || 'msedge', headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 1000 }, permissions: ['clipboard-read', 'clipboard-write'] });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  const ready = name => page.waitForFunction(name => !document.getElementById('client-guide').hidden && document.querySelector('.home-guide-head h2')?.textContent === name, name);
  try {
    const response = await page.goto(base + '/#opencode');
    assert.equal(response.headers()['content-security-policy'], undefined);
    assert.equal(response.headers()['x-frame-options'], undefined);
    assert.equal(response.headers()['x-content-type-options'], 'nosniff');
    assert.equal(response.headers()['referrer-policy'], 'strict-origin-when-cross-origin');
    await ready('OpenCode');
    assert.equal(await page.locator('#client-list a').count(), 9);
    const macroResult = await page.evaluate(() => {
      const tricky = "https://example.com/mcp?a='\"<&b=1";
      return {
        tricky, raw: HomeGuides.replaceMacros('{{mcp_url}}', tricky),
        json: HomeGuides.replaceMacros('{{mcp_url_json}}', tricky),
        shell: HomeGuides.replaceMacros('{{mcp_url_shell}}', tricky),
        powershell: HomeGuides.replaceMacros('{{mcp_url_powershell}}', tricky),
        once: HomeGuides.replaceMacros('{{mcp_url}}', '{{unknown}}')
      };
    });
    assert.equal(macroResult.raw, macroResult.tricky);
    assert.equal(JSON.parse(macroResult.json), macroResult.tricky);
    assert.equal(macroResult.shell, "'" + macroResult.tricky.replaceAll("'", "'\"'\"'") + "'");
    assert.equal(macroResult.powershell, "'" + macroResult.tricky.replaceAll("'", "''") + "'");
    assert.equal(macroResult.once, '{{unknown}}');
    const specialRender = await page.evaluate(() => {
      const value = "https://example.com/mcp?a='\"<img onerror=alert(1)>&b=1";
      const node = document.createElement('div');
      node.innerHTML = HomeGuides.render('{{mcp_url}}\n\n`{{mcp_url}}`\n\n```json\n{"url": {{mcp_url_json}}}\n```', value, location.href);
      return { value, text: node.querySelector('p').textContent, inline: node.querySelector('p code').textContent, json: node.querySelector('pre code').textContent, images: node.querySelectorAll('img').length };
    });
    assert.equal(specialRender.text, specialRender.value);
    assert.equal(specialRender.inline, specialRender.value);
    assert.equal(JSON.parse(specialRender.json).url, specialRender.value);
    assert.equal(specialRender.images, 0);
    const rejected = await page.evaluate(() => {
      const item = { id: 'test', name: 'Test', icon: '/assets/icons/openai.svg', guide: 'test.md' };
      return [[], [item, item], [{ ...item, guide: '../test.md' }], [{ ...item, guide: 'https://evil.example/a.md' }], [{ ...item, icon: 'javascript:alert(1)' }], [{ ...item, guide: '%2e%2e/test.md' }]].map(raw => {
        try { HomeGuides.validateIndex(raw, location.origin); return false; } catch { return true; }
      });
    });
    assert.ok(rejected.every(Boolean));
    const unsafe = await page.evaluate(() => HomeGuides.render('<img src=x onerror="alert(1)">\n\n[x](javascript:alert(1))\n\n![pic](images/test.png)\n\n[x](more.md)', 'https://example.com/<img onerror=alert(1)>', location.origin + '/assets/home/guides/test.md'));
    assert.ok(!/<img[^>]*onerror|href="javascript:/i.test(unsafe));
    assert.ok(unsafe.includes('/assets/home/guides/images/test.png') && unsafe.includes('/assets/home/guides/more.md'));
    assert.ok(await page.evaluate(() => { try { HomeGuides.render('{{unknown}}', 'url', location.href); return false; } catch { return true; } }));
    assert.ok(await page.evaluate(() => { try { HomeGuides.render(':::: details Outer\n::: details Inner\nx\n:::\n::::', 'url', location.href); return false; } catch { return true; } }));
    for (const client of index) {
      await page.locator('#client-list a[data-client="' + client.id + '"]').click();
      await ready(client.name);
      assert.equal(await page.locator('#client-list a[aria-current]').getAttribute('data-client'), client.id);
      const texts = await page.locator('#client-guide pre code').allTextContents();
      assert.equal(await page.locator('#client-guide ol > li').count(), expectedCounts[client.id][0]);
      assert.equal(texts.length, expectedCounts[client.id][1]);
      assert.ok(texts.every(text => !text.includes('{{')));
      for (const text of texts.filter(text => text.startsWith('{'))) assert.ok(JSON.parse(text));
      assert.equal(await page.locator('#client-guide a[target="_blank"]').getAttribute('rel'), 'noopener noreferrer');
      if (client.id === 'cursor') await page.locator('#client-guide details summary').click();
      await page.locator('#client-guide button[data-copy-code]').first().click();
      assert.equal((await page.evaluate(() => navigator.clipboard.readText())).replace(/\r\n/g, '\n'), texts[0]);
    }
    await page.goto(base + '/#opencode'); await ready('OpenCode');
    const details = page.locator('#client-guide details');
    await details.locator('summary').click(); assert.equal(await details.getAttribute('open'), '');
    await page.locator('#client-list a[data-client="codex"]').focus(); await page.keyboard.press('Enter'); await ready('Codex');
    await page.evaluate(() => Object.defineProperty(navigator.clipboard, 'writeText', { configurable: true, value: () => Promise.reject(new Error('denied')) }));
    await page.locator('#client-guide button[data-copy-code]').click();
    await page.waitForFunction(() => document.getElementById('copy-status').textContent.includes('手动复制'));
    assert.ok((await page.evaluate(() => getSelection().toString())).includes(endpoint));
    await page.goto(base + '/#unknown'); await ready('Cherry Studio');
    const screenshotDir = process.env.HOME_TEST_SCREENSHOTS;
    await page.evaluate(() => { document.getElementById('copy-status').textContent = ''; getSelection().removeAllRanges(); });
    if (screenshotDir) { fs.mkdirSync(screenshotDir, { recursive: true }); await page.screenshot({ path: path.join(screenshotDir, 'home-desktop.png'), fullPage: true }); }
    await page.setViewportSize({ width: 390, height: 844 });
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth));
    if (screenshotDir) await page.screenshot({ path: path.join(screenshotDir, 'home-mobile.png'), fullPage: true });
    // Failed requests are retriable and are not cached.
    let fail = true;
    await page.route('**/codex.md', route => fail ? route.fulfill({ status: 503, body: 'unavailable' }) : route.continue());
    await page.goto(base + '/?scenario=retry#codex'); await page.locator('#guide-retry').waitFor({ state: 'visible' });
    assert.ok((await page.locator('#guide-status').textContent()).includes('503'));
    fail = false; await page.locator('#guide-retry').click(); await ready('Codex');
    await page.unroute('**/codex.md');
    // Delayed obsolete request cannot overwrite a newer selection.
    let release;
    const gate = new Promise(resolve => { release = resolve; });
    let started;
    const received = new Promise(resolve => { started = resolve; });
    await page.route('**/opencode.md', async route => { started(); await gate; await route.continue(); });
    await page.goto(base + '/?scenario=race#opencode'); await received;
    await page.locator('#client-list a[data-client="codex"]').click(); await ready('Codex');
    const obsoleteResponse = page.waitForResponse('**/opencode.md'); release();
    await obsoleteResponse;
    await page.waitForFunction(() => document.getElementById('guide-status').textContent === '');
    assert.equal(await page.locator('.home-guide-head h2').textContent(), 'Codex');
    await page.unroute('**/opencode.md');
    // Index failure and missing icon fallback.
    await page.route('**/index.json', route => route.fulfill({ status: 200, contentType: 'application/json', body: '{invalid' }));
    await page.goto(base); await page.locator('#guide-retry').waitFor({ state: 'visible' });
    await page.unroute('**/index.json'); await page.locator('#guide-retry').click(); await ready('Cherry Studio');
    await page.route('**/cherry-studio.png', route => route.fulfill({ status: 404 }));
    await page.reload(); await ready('Cherry Studio');
    await page.waitForFunction(() => document.querySelector('#client-list a img').src.endsWith('/openai.svg'));
    await page.unroute('**/cherry-studio.png');
    const framePage = await context.newPage();
    await framePage.goto(base);
    await framePage.evaluate(base => { const frame = document.createElement('iframe'); frame.src = base + '/#cursor'; document.body.append(frame); }, base);
    await framePage.frameLocator('iframe').locator('.home-guide-head h2').waitFor();
    assert.equal(await framePage.frameLocator('iframe').locator('.home-guide-head h2').textContent(), 'Cursor');
    await framePage.close();
    const noJS = await browser.newContext({ javaScriptEnabled: false });
    const staticPage = await noJS.newPage(); await staticPage.goto(base);
    assert.ok((await staticPage.locator('noscript').textContent()).includes('请启用 JavaScript'));
    assert.equal(await staticPage.locator('#endpoint').textContent(), endpoint);
    await noJS.close();
    assert.deepEqual(errors, []);
    console.log('PASS: nine guides, macros, sanitization, paths, folding, copy/fallback, retry, race, responsive layout, icons, iframe and noscript');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
