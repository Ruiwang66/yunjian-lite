"""Run as root on a Docker host to register and install this host's Agent.
Uses the initial password from the private data volume; never prints credentials.
"""
import argparse,http.cookiejar,json,os,pwd,grp,subprocess,urllib.request
from pathlib import Path
parser=argparse.ArgumentParser()
parser.add_argument('--server',default='http://127.0.0.1:5050')
parser.add_argument('--ip',required=True)
parser.add_argument('--name',default='Debian · 主控服务器')
args=parser.parse_args()
root=Path(__file__).resolve().parent.parent
container=subprocess.check_output(['docker','compose','ps','-q','server'],cwd=root,text=True).strip()
inspection=json.loads(subprocess.check_output(['docker','inspect',container],text=True))[0]
mount=next(m for m in inspection['Mounts'] if m['Destination']=='/app/data')
password_file=Path(mount['Source'])/'admin-password.txt'
password=password_file.read_text().split('password: ')[1].strip()
jar=http.cookiejar.CookieJar();client=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
def call(path,data=None):
 body=json.dumps(data).encode() if data is not None else None
 return json.loads(client.open(urllib.request.Request(args.server+'/api'+path,data=body,headers={'Content-Type':'application/json'}),timeout=15).read())
call('/auth/login',{'username':'admin','password':password});del password
try:user=pwd.getpwnam('cloudease-agent')
except KeyError:
 subprocess.run(['useradd','--system','--no-create-home','--shell','/usr/sbin/nologin','cloudease-agent'],check=True)
 user=pwd.getpwnam('cloudease-agent')
config=Path('/etc/cloudease-lite');config.mkdir(mode=0o750,exist_ok=True);os.chown(config,0,user.pw_gid);os.chmod(config,0o750)
token_file=config/'agent-token'
servers=call('/servers');existing=next((s for s in servers if s['name']==args.name),None)
if not existing:
 result=call('/servers',{'name':args.name});identifier=result['server']['id'];token=result['token']
 token_file.write_text(token);del token
elif not token_file.exists():
 identifier=existing['id'];result=call(f'/servers/{identifier}/token',{});token_file.write_text(result['token'])
else:identifier=existing['id']
os.chown(token_file,0,user.pw_gid);os.chmod(token_file,0o640)
env_file=config/'agent.env';env_file.write_text(f'CLOUDEASE_SERVER={args.server}\nCLOUDEASE_IP={args.ip}\n');os.chown(env_file,0,user.pw_gid);os.chmod(env_file,0o640)
subprocess.run(['install','-m','0755',str(root/'bin/downloads/cloud-agent-linux-amd64'),str(root/'cloud-agent')],check=True)
subprocess.run(['install','-m','0644',str(root/'deploy/cloud-agent.service'),'/etc/systemd/system/cloud-agent.service'],check=True)
subprocess.run(['systemctl','daemon-reload'],check=True)
subprocess.run(['systemctl','enable','--now','cloud-agent'],check=True)
call('/auth/logout',{})
print(json.dumps({'server_id':identifier,'agent_service':'cloud-agent','installed':True}))
