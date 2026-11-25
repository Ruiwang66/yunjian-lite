// Optional real-browser regression against scripts/local-v03-preview.go only.
// This uses an isolated headless context, never a user's browser profile.
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const testPassword = process.env.CLOUDEASE_TEST_PASSWORD;
assert.ok(testPassword, 'Set CLOUDEASE_TEST_PASSWORD for the loopback fixture');
const root = path.resolve(__dirname, '..');
const { chromium } = require(path.join(root, '.tools/browser/node_modules/playwright-core'));
const origin = process.env.CLOUDEASE_TEST_ORIGIN || 'http://127.0.0.1:5059';
const url = new URL(origin);
assert.equal(url.hostname, '127.0.0.1', 'Only the explicitly created loopback fixture is supported');
const browserPath = process.env.CLOUDEASE_TEST_BROWSER || 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe';
const output = path.join(root, '.runtime');
const report = { origin, browser: '', checks: [], pageErrors: [], externalRequests: [] };
let browser;

async function context() {
  const ctx = await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:'reduce'});
  await ctx.route('**/*', route => {
    if (new URL(route.request().url()).origin !== origin) {
      report.externalRequests.push(new URL(route.request().url()).origin);
      return route.abort();
    }
    return route.continue();
  });
  const page = await ctx.newPage();
  page.on('pageerror', error => report.pageErrors.push(error.message));
  return {ctx,page};
}

async function login(page, role) {
  await page.goto(origin+'/login');
  await page.getByPlaceholder('admin',{exact:true}).fill(role);
  await page.getByPlaceholder('输入账户密码').fill(testPassword);
  await page.locator('button[type=submit]').click();
  await page.waitForURL('**/admin');
  await page.locator('.map-canvas svg').waitFor();
}

(async () => {
  browser = await chromium.launch({executablePath:browserPath,headless:true,args:['--no-proxy-server','--disable-background-networking']});
  report.browser = browser.version();
  const publicView = await context();
  await publicView.page.goto(origin+'/');
  await publicView.page.getByRole('button',{name:'展开或收起地图',exact:true}).waitFor();
  await publicView.page.getByRole('button',{name:'展开或收起地图',exact:true}).click();
  await publicView.page.locator('.map-canvas svg').waitFor();
  await publicView.page.locator('.map-regions button').first().click();
  await publicView.page.locator('.map-selection').waitFor();
  assert.equal(await publicView.page.getByRole('button',{name:'设置地区',exact:true}).count(),0);
  await publicView.page.screenshot({path:path.join(output,'v06-regression-public-map.png'),fullPage:true});
  const publicData = await (await publicView.ctx.request.get(origin+'/api/public/servers')).json();
  assert.ok(publicData.length>0);
  const managedID=publicData.find(s=>s.status==='online')?.id;
  assert.ok(managedID, 'online simulated Linux node missing');
  assert.ok(publicData.every(s=>!('ip' in s)&&!('token' in s)&&!('management_json' in s)));
  report.checks.push('public map expansion, region selection and explicit public projection');
  await publicView.ctx.close();

  const admin = await context();
  await admin.page.goto(origin+'/login');
  await admin.page.locator('.login-card').waitFor();
  await admin.page.screenshot({path:path.join(output,'v06-regression-login.png'),fullPage:true});
  await login(admin.page,'admin');
  await admin.page.getByRole('button',{name:'设置地区',exact:true}).waitFor();
  await admin.page.screenshot({path:path.join(output,'v06-regression-admin-map.png'),fullPage:true});
  report.checks.push('login card, admin navigation and admin map settings entry');

  // Add only a disposable local fixture record; no production data is used.
  const dangerousName='<img src=x onerror="fixture"> literal name';
  const added=await admin.ctx.request.post(origin+'/api/servers',{data:{name:dangerousName,country_code:'US'}});
  assert.equal(added.status(),201);
  await admin.page.goto(origin+'/servers');
  await admin.page.getByText(dangerousName,{exact:true}).first().waitFor();
  assert.equal(await admin.page.locator('img[onerror]').count(),0);
  report.checks.push('untrusted server name rendered as literal text');

  await admin.page.goto(origin+'/security?server_id='+managedID);
  const run=admin.page.getByRole('button',{name:'执行命令',exact:true}).nth(3);
  await run.waitFor();
  await admin.page.waitForFunction(()=>!document.querySelectorAll('.command-grid button')[3]?.disabled);
  await run.click();
  await admin.page.getByRole('button',{name:'执行',exact:true}).click();
  const logs=admin.page.locator('.command-output');
  await logs.filter({hasText:'本地日志模拟'}).waitFor({timeout:20000});
  const metrics=await logs.evaluate(el=>({text:el.textContent.length,scripts:el.querySelectorAll('script').length,overflow:getComputedStyle(el).overflowY,scrollbar:getComputedStyle(el).scrollbarWidth,tab:el.tabIndex,client:el.clientHeight,total:el.scrollHeight}));
  assert.ok(metrics.text>1000&&metrics.scripts===0&&metrics.total>metrics.client);
  assert.equal(metrics.overflow,'auto');assert.equal(metrics.scrollbar,'none');assert.equal(metrics.tab,0);
  await logs.focus();await admin.page.keyboard.press('PageDown');
  await admin.page.waitForFunction(()=>document.querySelector('.command-output').scrollTop>0);
  await logs.hover();await admin.page.mouse.wheel(0,400);
  await admin.page.screenshot({path:path.join(output,'v06-regression-command-scroll.png'),fullPage:true});
  report.command=metrics;
  report.checks.push('bounded long output, escaped markup, hidden scrollbar, wheel and keyboard scrolling');

  for(let i=0;i<3;i++) {
    await admin.page.goto(origin+'/logs');await admin.page.getByRole('heading',{name:'系统日志',exact:true}).waitFor();
    await admin.page.goto(origin+'/admin');await admin.page.locator('.map-canvas svg').waitFor();
  }
  report.checks.push('repeated log/dashboard mount and unmount without page exceptions');
  await admin.ctx.close();

  const viewer=await context();await login(viewer.page,'viewer');
  assert.equal(await viewer.page.getByRole('button',{name:'设置地区',exact:true}).count(),0);
  assert.equal(await viewer.page.getByRole('link',{name:/安全中心/}).count(),0);
  await viewer.page.goto(origin+'/security?server_id='+managedID);
  await viewer.page.getByRole('button',{name:'执行命令',exact:true}).first().waitFor();
  assert.ok(await viewer.page.getByRole('button',{name:'执行命令',exact:true}).first().isDisabled());
  assert.equal((await viewer.ctx.request.post(origin+'/api/servers',{data:{name:'must refuse'}})).status(),403);
  report.checks.push('viewer sees existing read-only workspace and cannot edit regions or execute commands');
  await viewer.ctx.close();
  assert.deepEqual(report.pageErrors,[]);assert.deepEqual(report.externalRequests,[]);
  report.status='passed';
})().catch(error=>{report.status='failed';report.error=error.message;process.exitCode=1;}).finally(async()=>{
  await browser?.close();
  fs.writeFileSync(path.join(output,'v06-regression-browser-report.json'),JSON.stringify(report,null,2));
  console.log(JSON.stringify(report,null,2));
});
