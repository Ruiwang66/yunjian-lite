const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {chromium}=require('../.tools/browser/node_modules/playwright-core');
const origin=process.env.CLOUDEASE_TEST_ORIGIN||'http://127.0.0.1:5059',out=path.resolve(__dirname,'../.runtime/v06');
assert.equal(new URL(origin).hostname,'127.0.0.1');
let browser;const report={checks:[],errors:[]};
(async()=>{
 browser=await chromium.launch({executablePath:process.env.CLOUDEASE_TEST_BROWSER||'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',headless:true,args:['--no-proxy-server','--disable-background-networking']});
 const ctx=await browser.newContext({viewport:{width:1440,height:900},storageState:{cookies:[],origins:[{origin,localStorage:[{name:'cloudease.appearance.v1',value:JSON.stringify({scheme:'dark',accent:'blue'})}]}]}});
 await ctx.route('**/*',r=>new URL(r.request().url()).origin!==origin?r.abort():r.continue());
 const page=await ctx.newPage();page.on('pageerror',e=>report.errors.push(e.message));
 await page.addInitScript(()=>{window.firstFrames=[];const start=performance.now();function frame(){window.firstFrames.push({time:performance.now(),theme:document.documentElement.dataset.theme,background:getComputedStyle(document.documentElement).backgroundColor});if(performance.now()-start<3000)requestAnimationFrame(frame)}requestAnimationFrame(frame)});
 const cdp=await ctx.newCDPSession(page);await cdp.send('Network.enable');await cdp.send('Network.emulateNetworkConditions',{offline:false,latency:100,downloadThroughput:300000,uploadThroughput:300000});
 await page.goto(origin+'/login');await page.getByPlaceholder('admin',{exact:true}).waitFor();report.firstFrames=await page.evaluate(()=>window.firstFrames);assert.ok(report.firstFrames.length>0);assert.ok(report.firstFrames.every(f=>f.theme==='dark'));await cdp.send('Network.emulateNetworkConditions',{offline:false,latency:0,downloadThroughput:-1,uploadThroughput:-1});
 const boot=await ctx.request.get(origin+'/assets/appearance-init.js');assert.equal(boot.status(),200);assert.match(boot.headers()['content-type'],/javascript/);report.checks.push('throttled dark cold start: all observed animation frames dark; same-origin classic script is executable');
 await page.goto(origin+'/');await page.locator('.node-card').first().waitFor();let sockets=0,requests=0;page.on('websocket',()=>sockets++);page.on('request',r=>{if(new URL(r.url()).pathname==='/api/public/servers')requests++});
 await page.getByRole('button',{name:'外观设置',exact:true}).click();
 const controlCtx=await browser.newContext({viewport:{width:1440,height:900}});const control=await controlCtx.newPage();await control.goto(origin+'/');await control.locator('.node-card').first().waitFor();let controlRequests=0;control.on('request',r=>{if(new URL(r.url()).pathname==='/api/public/servers')controlRequests++});
 const before=Date.now();await page.evaluate(()=>new Promise(resolve=>{const start=performance.now();let n=0;const buttons=[...document.querySelectorAll('.scheme-options button')];const timer=setInterval(()=>{buttons[n++%2].click();if(performance.now()-start>=60000){clearInterval(timer);resolve()}},1000)}));
 report.connection={seconds:(Date.now()-before)/1000,newSockets:sockets,listRequests:requests,controlListRequests:controlRequests};assert.equal(sockets,0);assert.ok(requests>=12 && requests<=35);assert.ok(Math.abs(requests-controlRequests)<=3);await controlCtx.close();await page.keyboard.press('Escape');report.checks.push('60 seconds of theme switches preserve the existing websocket and four-second poll cadence');
 await page.getByRole('button',{name:'外观设置',exact:true}).click();
 report.glass=[];for(const name of ['关闭','轻盈','通透']){await page.getByRole('button',{name,exact:true}).click();const style=await page.locator('.public-header').evaluate(el=>({filter:getComputedStyle(el).backdropFilter,background:getComputedStyle(el).backgroundColor}));report.glass.push({name,...style});assert.equal(style.filter,name==='关闭'?'none':name==='轻盈'?'blur(10px)':'blur(16px)')}
 // Remove only the feature-gated rules to simulate an engine without filter support.
 await page.evaluate(()=>{for(const sheet of document.styleSheets){try{for(let i=sheet.cssRules.length-1;i>=0;i--){const rule=sheet.cssRules[i];if(rule.type===CSSRule.SUPPORTS_RULE && rule.conditionText.includes('backdrop-filter'))sheet.deleteRule(i)}}catch{}}});
 const fallback=await page.locator('.public-header').evaluate(el=>({filter:getComputedStyle(el).backdropFilter,background:getComputedStyle(el).backgroundColor}));assert.equal(fallback.filter,'none');assert.match(fallback.background,/^rgb\(/);report.checks.push('off/light/clear filter values and simulated unsupported-engine solid fallback');
 await ctx.close();assert.deepEqual(report.errors,[]);report.status='passed';
})().catch(e=>{report.status='failed';report.error=e.stack;process.exitCode=1}).finally(async()=>{if(browser)await browser.close();fs.writeFileSync(path.join(out,'lifecycle-report.json'),JSON.stringify(report,null,2));console.log(JSON.stringify(report,null,2))});
