"""Verify an already deployed control plane in-place; do not export credentials.
Run from the project Docker host. Uses credentials only for same-host API checks.
"""
import base64,hashlib,http.cookiejar,json,os,socket,struct,subprocess,time,urllib.request
from pathlib import Path
root=Path(__file__).resolve().parent.parent
container=subprocess.check_output(['docker','compose','ps','-q','server'],cwd=root,text=True).strip()
info=json.loads(subprocess.check_output(['docker','inspect',container],text=True))[0]
source=next(m['Source'] for m in info['Mounts'] if m['Destination']=='/app/data')
password=(Path(source)/'admin-password.txt').read_text().split('password: ')[1].strip()
base='http://127.0.0.1:5050';jar=http.cookiejar.CookieJar();http=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
def call(path,data=None):
 req=urllib.request.Request(base+path,data=json.dumps(data).encode() if data is not None else None,headers={'Content-Type':'application/json'})
 return json.loads(http.open(req,timeout=15).read())
assert call('/healthz')['status']=='ok'
call('/api/auth/login',{'username':'admin','password':password});del password
servers=call('/api/servers');assert len(servers)>=1
server=servers[0];assert server['status']=='online' and server['os']=='linux'
assert server['latest']['memory_total']>0 and 0<=server['latest']['cpu']<=100
history=call(f"/api/servers/{server['id']}/metrics?hours=1");assert len(history)>=1
logs=call('/api/logs?limit=100')['items'];assert any(l['message']=='Agent connected' for l in logs)
key=base64.b64encode(os.urandom(16)).decode();cookie='; '.join(f'{c.name}={c.value}' for c in jar)
with socket.create_connection(('127.0.0.1',5050),timeout=40) as sock:
 sock.sendall((f'GET /api/ws HTTP/1.1\r\nHost: 127.0.0.1:5050\r\nOrigin: {base}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\nCookie: {cookie}\r\n\r\n').encode())
 header=b''
 while not header.endswith(b'\r\n\r\n'):header+=sock.recv(1)
 assert b'101 Switching Protocols' in header
 def receive(n):
  data=b''
  while len(data)<n:
   piece=sock.recv(n-len(data))
   if not piece:raise RuntimeError('WebSocket closed early')
   data+=piece
  return data
 def frame():
  first,second=receive(2);size=second&127
  if size==126:size=struct.unpack('!H',receive(2))[0]
  elif size==127:size=struct.unpack('!Q',receive(8))[0]
  return first&15,receive(size)
 def pong(data):
  mask=os.urandom(4);sock.sendall(bytes([0x8a,0x80|len(data)])+mask+bytes(v^mask[i%4] for i,v in enumerate(data)))
 connected=False;updated=False;deadline=time.monotonic()+40
 while time.monotonic()<deadline:
  opcode,data=frame()
  if opcode==9:pong(data);continue
  if opcode!=1:continue
  message=json.loads(data)
  if message['type']=='connected':connected=True
  if message['type']=='metrics_updated':updated=True;break
 assert connected and updated
updated_server=call(f"/api/servers/{server['id']}")
assert updated_server['latest']['id']>server['latest']['id']
assert updated_server['status']=='online'
call('/api/auth/logout',{})
print(json.dumps({'health':'ok','server_id':server['id'],'os':server['platform'],'online':True,'history_samples':len(history),'initial_metric_id':server['latest']['id'],'new_metric_id':updated_server['latest']['id'],'websocket_connected':connected,'websocket_live_update':updated,'log_entries':len(logs)},ensure_ascii=False))
