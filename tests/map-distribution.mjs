import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
const countries=JSON.parse(fs.readFileSync('web/src/components/countries.json','utf8'));
const geometry=JSON.parse(fs.readFileSync('web/src/components/world-map.json','utf8'));
const source=fs.readFileSync('web/src/geography.js','utf8').replace(/^import .*;\r?\n/,'').replaceAll('export ','');
const context={data:countries};vm.createContext(context);vm.runInContext(source+'\nglobalThis.calculate=distribution;',context);
const result=context.calculate([{id:1,country_code:'CN',status:'online'},{id:2,country_code:'CN',status:'offline'},{id:3,country_code:'US',status:'pending'},{id:4,country_code:'',status:'online'},{id:5,country_code:'ZZ',status:'online'}]);
assert.equal(result.groups.length,2);assert.equal(result.unknown,2);assert.equal(result.groups[0].total,2);assert.equal(result.groups[0].online,1);
const codes=new Set(countries.map(c=>c.code));assert.equal(codes.size,countries.length);
const backendCodes=fs.readFileSync('shared/countries.go','utf8').match(/strings\.Fields\("([A-Z ]+)"\)/)[1].split(' ');assert.deepEqual(new Set(backendCodes),codes);
assert.deepEqual(new Set(geometry.regions.map(r=>r.code)),codes);
for(const r of geometry.regions){assert.ok(Number.isFinite(r.x)&&Number.isFinite(r.y));assert.ok(r.x>=0&&r.x<=1000&&r.y>=0&&r.y<=520);assert.ok(r.path||r.tiny)}
for(const code of ['CN','US','SG','HK','TW','DE','AU'])assert.ok(codes.has(code));
assert.equal(context.calculate([]).groups.length,0);
console.log('MAP_DISTRIBUTION_OK: country grouping, online counts, unknown regions, map/code consistency and empty state.');
