const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),zlib=require('node:zlib');
const {chromium}=require('../.tools/browser/node_modules/playwright-core');
const root=path.resolve(__dirname,'..'),out=path.join(root,'.runtime/v06');
const v05='http://127.0.0.1:5058',v06=process.env.CLOUDEASE_TEST_ORIGIN||'http://127.0.0.1:5059';
const samples=Array.from({length:8},(_,i)=>({id:100+i,name:`模拟节点 ${i+1}`,country_code:i%2?'HK':'CN',status:'online',platform:'Ubuntu',os:'linux',arch:'amd64',cpu_cores:2,uptime:4000000,last_seen:new Date().toISOString(),latest:{id:i+1,created_at:new Date().toISOString(),cpu:5+i,memory:11+i,disk:4+i,network_tx:937500000,network_rx:754200000,network_tx_rate:423+i*500,network_rx_rate:182+i*100,memory_used:207300000,memory_total:1800000000,disk_used:1700000000,disk_total:39100000000}}));
const report={conditions:{browser:'',viewport:'1440x900',nodes:8,secondsPerRun:15,runsPerMode:3,measurement:'headless Chromium requestAnimationFrame; >16.7ms slow; same browser/fixture, no network outside loopback'},runs:[],checks:[]};
let browser;
function resources(dir){const files=fs.readdirSync(path.join(dir,'assets')).filter(f=>/^index-.*\.(css|js)$/.test(f)||f==='appearance-init.js');return {files,gzipBytes:files.reduce((n,f)=>n+zlib.gzipSync(fs.readFileSync(path.join(dir,'assets',f))).length,0)}}
function luminance(rgb){return rgb.map(v=>v/255).map(v=>v<=.04045?v/12.92:((v+.055)/1.055)**2.4).reduce((n,v,i)=>n+v*[.2126,.7152,.0722][i],0)}
function ratio(a,b){const x=luminance(a),y=luminance(b);return (Math.max(x,y)+.05)/(Math.min(x,y)+.05)}
(async()=>{
 browser=await chromium.launch({executablePath:process.env.CLOUDEASE_TEST_BROWSER||'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',headless:true,args:['--no-proxy-server','--disable-background-networking']});report.conditions.browser=browser.version();
 report.resources={v05:resources(path.join(root,'../github-v04-release-20261030/repository/web/dist')),v06:resources(path.join(root,'web/dist'))};report.resources.addedGzipBytes=report.resources.v06.gzipBytes-report.resources.v05.gzipBytes;assert.ok(report.resources.addedGzipBytes<=20480);
 if(process.env.CLOUDEASE_REUSE_SCROLL==='1') report.runs=JSON.parse(fs.readFileSync(path.join(out,'performance.json'),'utf8')).runs;
 else for(const mode of ['v05','off','light'])for(let run=1;run<=3;run++){
  const origin=mode==='v05'?v05:v06,ctx=await browser.newContext({viewport:{width:1440,height:900},recordVideo:{dir:path.join(out,'videos'),size:{width:1440,height:900}}});
  await ctx.route('**/*',r=>new URL(r.request().url()).origin!==origin?r.abort():r.continue());await ctx.route('**/api/public/servers',r=>r.fulfill({json:samples}));
  await ctx.addInitScript(glass=>localStorage.setItem('cloudease.appearance.v1',JSON.stringify({scheme:'light',accent:'violet',publicLayout:'modern',density:'comfortable',glass,motion:'system'})),mode==='v05'?'off':mode);
  const page=await ctx.newPage();await page.goto(origin+'/');await page.locator(mode==='v05'?'.server-card':'.node-card').first().waitFor();
  const measurement=await page.evaluate(()=>new Promise(resolve=>{const start=performance.now(),deltas=[];let previous;function frame(now){if(previous!==undefined)deltas.push(now-previous);previous=now;const extent=document.documentElement.scrollHeight-innerHeight;scrollTo(0,(Math.sin((now-start)/1200-Math.PI/2)+1)/2*extent);if(now-start<15000)requestAnimationFrame(frame);else resolve({frames:deltas.length,slowFrames:deltas.filter(d=>d>16.7).length,slowPercent:deltas.filter(d=>d>16.7).length/deltas.length*100,medianFrameMS:[...deltas].sort((a,b)=>a-b)[Math.floor(deltas.length/2)]})}requestAnimationFrame(frame)}));
  report.runs.push({mode,run,...measurement});await ctx.close();fs.writeFileSync(path.join(out,'performance-progress.json'),JSON.stringify(report,null,2));console.log(JSON.stringify({mode,run,...measurement}));
 }
 report.medians=Object.fromEntries(['v05','off','light'].map(mode=>{const values=report.runs.filter(r=>r.mode===mode).map(r=>r.slowPercent).sort((a,b)=>a-b);return [mode,values[1]]}));report.lightAddedSlowPercentagePoints=report.medians.light-report.medians.off;
 const ctx=await browser.newContext({viewport:{width:1440,height:900}}),page=await ctx.newPage();await page.goto(v06+'/login');
 report.contrast=[];
 for(const theme of ['light','dark'])for(const accent of ['graphite','blue','teal','violet']){
  const colors=await page.evaluate(({theme,accent})=>{Object.assign(document.documentElement.dataset,{theme,accent});const style=getComputedStyle(document.documentElement);return Object.fromEntries(['text','muted','surface','accent','accent-ink','success','success-bg','danger','danger-bg','down','down-bg'].map(k=>{const el=document.createElement('span');el.style.color=`var(--${k})`;document.body.append(el);const rgb=getComputedStyle(el).color.match(/[\d.]+/g).slice(0,3).map(Number);el.remove();return [k,rgb]}))},{theme,accent});
  for(const [foreground,background] of [['text','surface'],['muted','surface'],['accent','surface'],['accent-ink','accent'],['success','success-bg'],['danger','danger-bg'],['down','down-bg']]){const contrast=ratio(colors[foreground],colors[background]);report.contrast.push({theme,accent,foreground,background,ratio:contrast});assert.ok(contrast>=4.5,`${theme}/${accent}/${foreground}: ${contrast}`)}
 }
 await ctx.close();report.checks.push('first-screen extra gzip <=20KB; three 15-second runs per mode; 56 semantic color contrast pairs >=4.5');report.status='passed';
})().catch(e=>{report.status='failed';report.error=e.stack;process.exitCode=1}).finally(async()=>{if(browser)await browser.close();fs.writeFileSync(path.join(out,'performance.json'),JSON.stringify(report,null,2));console.log(JSON.stringify({status:report.status,addedGzipBytes:report.resources?.addedGzipBytes,medians:report.medians,lightAddedSlowPercentagePoints:report.lightAddedSlowPercentagePoints,error:report.error},null,2))});
