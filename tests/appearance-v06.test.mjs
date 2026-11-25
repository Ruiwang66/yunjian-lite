import test from 'node:test';
import assert from 'node:assert/strict';
import { defaults, parseAppearance, normalizeAppearance, normalizeDashboard, effectiveAppearance } from '../web/src/appearance-core.js';
import { sumField, appendRatePoint, sampleSignature, ratePath } from '../web/src/monitor-data.js';

test('node layout defaults to compact and migrates saved Modern preferences', () => {
  assert.equal(defaults.publicLayout, 'compact');
  const saved = parseAppearance(JSON.stringify({publicLayout:'modern',scheme:'dark',accent:'blue'}));
  assert.equal(saved.publicLayout, 'compact');
  assert.equal(saved.scheme, 'dark');
  assert.equal(saved.accent, 'blue');
});

test('invalid preference fields recover independently and reject unknown input', () => {
  assert.deepEqual(parseAppearance('{broken'),defaults);
  assert.deepEqual(parseAppearance('null'),defaults);
  const result=normalizeAppearance({scheme:'dark',accent:'script',density:'compact',extra:'ignored'});
  assert.equal(result.scheme,'dark');assert.equal(result.accent,'violet');assert.equal(result.density,'compact');assert.equal('extra' in result,false);
});
test('dashboard normalizes duplicate and missing modules without mutating input', () => {
  const input={sections:['activity','activity','secret'],mapExpanded:false};
  assert.deepEqual(normalizeDashboard(input),{sections:['activity','map'],mapExpanded:false});
  assert.equal(input.sections.length,3);
  assert.deepEqual(normalizeDashboard({sections:'map'}),{sections:['map','activity'],mapExpanded:true});
});
test('system appearance and reduction are separate from the saved user choice', () => {
  assert.deepEqual(effectiveAppearance({...defaults,scheme:'system'},true,true),{theme:'dark',motion:'reduced'});
  assert.deepEqual(effectiveAppearance({...defaults,scheme:'light'},true,false),{theme:'light',motion:'standard'});
  assert.equal(effectiveAppearance({...defaults,motion:'reduced'},false,false).motion,'reduced');
});
const servers=[{id:1,status:'online',latest:{id:10,created_at:'2026-11-11T00:00:00Z',network_tx:0,network_tx_rate:0,network_rx_rate:5}},{id:2,status:'offline',latest:{network_tx:500,network_tx_rate:900}},{id:3,status:'online',latest:null}];
test('totals include zero and offline traffic; live rates include online samples only', () => {
  assert.deepEqual(sumField(servers,'network_tx'),{value:500,covered:2,total:3});
  assert.deepEqual(sumField(servers,'network_tx_rate',true),{value:0,covered:1,total:2});
  assert.equal(sumField(servers,'missing').value,null);
});
test('rate history deduplicates identical reports, caps window and breaks long gaps', () => {
  const key=sampleSignature(servers);
  const first=appendRatePoint([],key,servers,100000);
  assert.equal(appendRatePoint(first,key,servers,104000),first);
  const next=appendRatePoint(first,key+'new',servers,104000);
  assert.match(ratePath(next,'rx',104000),/L/);
  const gap=appendRatePoint(next,key+'gap',servers,130000);
  assert.equal((ratePath(gap,'rx',130000).match(/M/g)||[]).length,2);
  assert.equal(appendRatePoint(gap,key+'later',servers,200000).length,1);
  assert.equal(ratePath(gap,'rx',300000),'');
});
