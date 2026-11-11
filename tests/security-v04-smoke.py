"""Non-destructive localhost acceptance against the explicitly labelled v0.4 fixture.

Run from the project root after starting the isolated 127.0.0.1:5054 preview.
Only localhost test accounts and disposable preview records are used.
"""
import http.cookiejar
import json
import os
import pathlib
import subprocess
import time
import urllib.error
import urllib.request

ORIGIN = 'http://127.0.0.1:5054'
ROOT = pathlib.Path(__file__).resolve().parents[1]
OUT = ROOT / '.runtime/v04-preview'
OUT.mkdir(parents=True, exist_ok=True)

def client():
    return urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()), urllib.request.ProxyHandler({}))

def call(opener, method, path, body=None, key=None):
    headers = {'Content-Type': 'application/json'}
    if key:
        headers['Authorization'] = 'Bearer ' + key
    req = urllib.request.Request(ORIGIN + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
    try:
        with opener.open(req, timeout=10) as resp:
            return resp.status, json.loads(resp.read())
    except urllib.error.HTTPError as err:
        return err.code, json.loads(err.read())

accounts = {}
for role in ('admin', 'operator', 'viewer'):
    opener = client()
    status, _ = call(opener, 'POST', '/api/auth/login', {'username': role, 'password': 'CloudEase04!Test'})
    assert status == 200, (role, status)
    accounts[role] = opener
admin, operator, viewer = (accounts[r] for r in ('admin', 'operator', 'viewer'))
status, servers = call(admin, 'GET', '/api/servers')
assert status == 200
server = next(s for s in servers if 'Linux 检测输入模拟' in s['name'])
sid = server['id']
for endpoint in ('overview', 'findings', 'alerts', 'events', 'rules', 'scores', 'servers'):
    assert call(viewer, 'GET', f'/api/security/{endpoint}?server_id={sid}')[0] == 403
assert call(operator, 'POST', '/api/security/scans', {'server_id': sid, 'path': '../../etc/shadow'})[0] == 400

status, scan = call(operator, 'POST', '/api/security/scans', {'server_id': sid})
assert status == 202, (status, scan)
deadline = time.monotonic() + 50
while time.monotonic() < deadline:
    status, report = call(operator, 'GET', f'/api/security/reports/{scan["id"]}?server_id={sid}')
    assert status == 200
    if report['scan']['status'] == 'success':
        break
    assert report['scan']['status'] in ('queued', 'running'), report
    time.sleep(.5)
else:
    raise AssertionError('scan never completed')
assert '本地安全测试夹具' in report['report']['scope']
(OUT / 'smoke-report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
status, overview = call(operator, 'GET', f'/api/security/overview?server_id={sid}')
assert status == 200 and overview['latest_score']['score'] is not None
initial_score = overview['latest_score']['score']
status, items = call(operator, 'GET', f'/api/security/findings?server_id={sid}')
finding = next(f for f in items['items'] if f['rule'] == 'ssh.root_password')
assert call(operator, 'PATCH', f'/api/security/findings/{finding["id"]}', {'server_id': sid, 'status': 'false_positive'})[0] == 200
assert call(operator, 'GET', f'/api/security/overview?server_id={sid}')[1]['latest_score']['score'] == initial_score
for endpoint in ('scans', 'scores', 'findings', 'events', 'alerts'):
    assert call(operator, 'GET', f'/api/security/{endpoint}?server_id={sid}&severity=high')[0] == 200, endpoint

def preview_server(name, country):
    status, existing = call(admin, 'GET', '/api/servers')
    row = next((r for r in existing if r['name'] == name), None)
    if row:
        sid = row['id']
        status, data = call(admin, 'POST', f'/api/servers/{sid}/token', {})
        assert call(admin, 'PATCH', f'/api/servers/{sid}/location', {'country_code':country})[0] == 200
    else:
        status, data = call(admin, 'POST', '/api/servers', {'name': name, 'country_code': country})
        # Creation response includes the server object and one-time key.
        sid = data.get('server', data).get('id')
    assert status in (200, 201) and sid and data.get('token'), (status, data.keys())
    return sid, data['token']

other, other_key = preview_server('本地权限测试 · 普通监控（模拟）', 'JP')
sample = {'os':'linux','platform':'debian','platform_version':'兼容模式输入模拟','arch':'amd64','cpu_cores':1,'sent_at':int(time.time()*1000),'nonce':'C'*22,'management':{'version':'0.4.0','enabled':False,'security_enabled':False,'file_roots':[],'services':[]}}
assert call(client(), 'POST', '/api/agent/report', sample, other_key)[0] == 200
assert call(operator, 'POST', '/api/security/scans', {'server_id':other})[0] == 409
assert call(operator, 'GET', f'/api/security/reports/{scan["id"]}?server_id={other}')[0] == 404
public = call(client(), 'GET', '/api/public/servers')[1]
assert 'security_enabled' not in json.dumps(public) and 'grade_json' not in json.dumps(public)

windows_id, windows_key = preview_server('本机 Windows · 真实只读监控样本', '')
binary = ROOT / 'bin/downloads/cloud-agent.exe'
env = dict(os.environ, CLOUDEASE_TOKEN=windows_key)
env.pop('CLOUDEASE_TOKEN_FILE', None)
_, before_samples = call(admin, 'GET', f'/api/servers/{windows_id}/metrics?hours=1')
before_ids = {item['id'] for item in before_samples}
with (OUT / 'real-windows-client.log').open('w', encoding='utf-8') as logfile:
    process = subprocess.Popen([str(binary), '--server', ORIGIN, '--name', '本机 Windows · 真实只读监控样本'], env=env, stdout=logfile, stderr=subprocess.STDOUT, creationflags=getattr(subprocess, 'CREATE_NO_WINDOW', 0))
    try:
        time.sleep(13)
        assert process.poll() is None, 'Windows monitor exited'
        status, metrics = call(admin, 'GET', f'/api/servers/{windows_id}/metrics?hours=1')
        new_samples=[item for item in metrics if item['id'] not in before_ids]
        assert status == 200 and len(new_samples) >= 3, '4-second real sampling missing'
        latest = call(admin, 'GET', f'/api/security/overview?server_id={windows_id}')[1]
        assert not latest['enabled']
    finally:
        process.terminate()
        process.wait(timeout=10)

receipt = {'local_only':True, 'fixture_scan':'success', 'score':initial_score, 'workflow_did_not_change_score':True, 'viewer_denied':True, 'compatibility_scan_denied':True, 'cross_server_report_denied':True, 'risk_filters':'passed', 'public_security_data_absent':True, 'new_real_windows_samples':len(new_samples), 'report':str(OUT/'smoke-report.json')}
(OUT / 'smoke-verification.json').write_text(json.dumps(receipt,ensure_ascii=False,indent=2),encoding='utf-8')
print(json.dumps(receipt,ensure_ascii=False))
