const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const testPassword=process.env.CLOUDEASE_TEST_PASSWORD;if(!testPassword)throw new Error('Set CLOUDEASE_TEST_PASSWORD for the loopback fixture');
const {chromium}=require('../.tools/browser/node_modules/playwright-core');
const origin=process.env.CLOUDEASE_TEST_ORIGIN||'http://127.0.0.1:5059';assert.equal(new URL(origin).hostname,'127.0.0.1');
const out=path.resolve(__dirname,'../.runtime/v06');let browser;
(async()=>{
 browser=await chromium.launch({executablePath:process.env.CLOUDEASE_TEST_BROWSER||'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',headless:true,args:['--no-proxy-server']});
 const ctx=await browser.newContext({viewport:{width:1440,height:900}}),page=await ctx.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
 const health=await (await ctx.request.get(origin+'/healthz')).json();assert.equal(health.version,'0.6.0');
 await page.goto(origin+'/login');await page.getByPlaceholder('admin',{exact:true}).fill('admin');await page.getByPlaceholder('输入账户密码').fill(testPassword);await page.locator('button[type=submit]').click();await page.waitForURL('**/admin');await page.goto(origin+'/settings');await page.getByRole('button',{name:'深色',exact:true}).click();
 await page.locator('.el-table th').first().waitFor();const colors=await page.locator('.el-table th').first().evaluate(el=>({color:getComputedStyle(el).color,background:getComputedStyle(el).backgroundColor}));assert.notEqual(colors.color,colors.background);
 // Element Plus overlays the input with its selected label; the wrapper is the
 // mouse target, while the input remains the keyboard-accessible combobox.
 await page.locator('.el-table .el-select__wrapper').first().click();await page.getByRole('listbox').waitFor();await page.screenshot({path:path.join(out,'settings-final.png'),fullPage:true});await page.keyboard.press('Escape');
 assert.deepEqual(errors,[]);fs.writeFileSync(path.join(out,'final-smoke.json'),JSON.stringify({status:'passed',health,tableHeader:colors,errors},null,2));console.log('FINAL_V06_SMOKE_OK');
})().catch(e=>{console.error(e);process.exitCode=1}).finally(async()=>{if(browser)await browser.close()});
