"""Interactive SSH deployment helper; passwords stay in process memory, never source files.
Set PYTHONPATH to the project-local .tools/python directory when invoking.
"""
import argparse, getpass, json, sys, os, tarfile, base64, hashlib, hmac
from pathlib import Path
import paramiko
parser=argparse.ArgumentParser()
parser.add_argument('--host',required=True)
parser.add_argument('--port',type=int,default=22)
parser.add_argument('--user',default='root')
parser.add_argument('--host-key-sha256', help='Trusted SHA256:... SSH host key fingerprint, verified independently')
args=parser.parse_args()
client=paramiko.SSHClient()
known=Path('.runtime/known_hosts');known.parent.mkdir(exist_ok=True)
if known.exists():client.load_host_keys(str(known))
class PinnedHostKey(paramiko.MissingHostKeyPolicy):
 def missing_host_key(self, client, hostname, key):
  fingerprint='SHA256:'+base64.b64encode(hashlib.sha256(key.asbytes()).digest()).decode().rstrip('=')
  if not args.host_key_sha256 or not hmac.compare_digest(fingerprint, args.host_key_sha256):
   raise paramiko.SSHException('Unknown host key. Verify its SHA256 fingerprint independently and use --host-key-sha256.')
  client.get_host_keys().add(hostname,key.get_name(),key)
client.load_system_host_keys()
client.set_missing_host_key_policy(PinnedHostKey())
password=getpass.getpass('SSH password: ')
client.connect(args.host,port=args.port,username=args.user,password=password,timeout=20,auth_timeout=20,look_for_keys=False,allow_agent=False)
del password
client.save_host_keys(str(known))
print('SSH_READY',flush=True)
for line in sys.stdin:
 try:
  request=json.loads(line)
  if request['action']=='close':break
  if request['action']=='put':
   with client.open_sftp() as sftp:
    sftp.put(request['local'],request['remote'])
    if 'mode' in request:sftp.chmod(request['remote'],int(request['mode'],8))
   print(json.dumps({'ok':True,'uploaded':request['remote']}),flush=True)
  elif request['action']=='get':
   with client.open_sftp() as sftp:sftp.get(request['remote'],request['local'])
   print(json.dumps({'ok':True,'downloaded':request['local']}),flush=True)
  elif request['action']=='exec':
   stdin,stdout,stderr=client.exec_command(request['command'],timeout=request.get('timeout',180))
   stdin.close()
   for output in iter(stdout.readline,''):print(output.rstrip(),flush=True)
   error=stderr.read().decode('utf-8',errors='replace')
   if error:print(error,flush=True)
   print('SSH_EXIT='+str(stdout.channel.recv_exit_status()),flush=True)
  else:raise ValueError('Unsupported action')
 except Exception as e:print(json.dumps({'error':str(e)}),flush=True)
client.close()
