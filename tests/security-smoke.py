"""Run release binaries against an isolated loopback fixture; no installed services or real database."""
import os, json, time, socket, subprocess, urllib.request, urllib.error, pathlib, tempfile, hashlib
root=pathlib.Path(__file__).resolve().parents[1]
runtime=root/".runtime";runtime.mkdir(exist_ok=True)
folder=pathlib.Path(tempfile.mkdtemp(prefix="security-smoke-",dir=runtime))
data=folder/"data"
flags=subprocess.CREATE_NO_WINDOW if os.name=="nt" else 0
server_path=root/"bin"/("cloud-server.exe" if os.name=="nt" else "cloud-server")
agent_path=root/"bin/downloads"/("cloud-agent.exe" if os.name=="nt" else "cloud-agent-linux-amd64")
with socket.socket() as sock:sock.bind(("127.0.0.1",0));port=sock.getsockname()[1]
origin="http://127.0.0.1:"+str(port)
env=os.environ.copy()
env.update({"LISTEN_ADDR":"127.0.0.1:"+str(port),"DATA_DIR":str(data),"WEB_DIR":str(root/"web/dist"),"PUBLIC_URL":"","COOKIE_SECURE":"false","TRUSTED_PROXY_CIDRS":"","ALLOW_LEGACY_REPORTS":"false"})
log=(folder/"server.log").open("wb")
proc=subprocess.Popen([str(server_path)],cwd=root/"bin",env=env,stdout=log,stderr=subprocess.STDOUT,creationflags=flags)
cookie=""
def call(path,body=None):
    payload=None if body is None else json.dumps(body).encode()
    headers={"Origin":origin,"Content-Type":"application/json"}
    if cookie:headers["Cookie"]=cookie
    request=urllib.request.Request(origin+path,data=payload,headers=headers)
    with urllib.request.urlopen(request,timeout=10) as response:
        raw=response.read()
        return response.status,response.headers,json.loads(raw)
try:
    deadline=time.monotonic()+10
    while True:
        try:status,headers,result=call("/healthz");break
        except (OSError,urllib.error.URLError):
            if time.monotonic()>deadline or proc.poll() is not None:raise RuntimeError("Isolated server failed to start")
            time.sleep(.1)
    assert status==200
    first=(data/"admin-password.txt").read_text().split("password: ")[1].strip()
    status,headers,result=call("/api/auth/login",{"username":"admin","password":first})
    cookie=headers["Set-Cookie"].split(";")[0]
    assert "HttpOnly" in headers["Set-Cookie"] and "SameSite=Strict" in headers["Set-Cookie"]
    status,headers,result=call("/api/servers",{"name":"isolated smoke fixture"})
    assert status==201
    key_path=data/"smoke-key";key_path.write_text(result["token"],encoding="utf-8")
    if os.name!="nt":key_path.chmod(0o600)
    attempt=subprocess.run([str(agent_path),"--server","http://monitor.invalid","--token-file",str(key_path),"--once"],capture_output=True,text=True,timeout=10,creationflags=flags)
    assert attempt.returncode!=0 and "require HTTPS" in attempt.stderr
    run=subprocess.run([str(agent_path),"--server",origin,"--token-file",str(key_path),"--once"],capture_output=True,text=True,timeout=25,creationflags=flags)
    assert run.returncode==0,run.stderr
    status,headers,result=call("/api/public/servers")
    assert len(result)==1 and result[0]["latest"] is not None
    assert "token" not in result[0] and "ip" not in result[0] and "last_report_nonce" not in result[0]
    assert headers["Cache-Control"]=="no-store"
    server_id=result[0]["id"]
    with urllib.request.urlopen(urllib.request.Request(origin+"/api/downloads/cloud-agent.exe/sha256",headers={"Cookie":cookie}),timeout=10) as response:
        checksum=response.read().decode().strip()
    assert checksum==hashlib.sha256((root/"bin/downloads/cloud-agent.exe").read_bytes()).hexdigest()
    status,headers,result=call("/api/public/servers/"+str(server_id)+"/metrics")
    assert len(result)==1
    (folder/"result.json").write_text(json.dumps({"status":"passed","scope":"isolated loopback release binaries","server_binary":str(server_path),"agent_binary":str(agent_path),"checks":["bootstrap and login","HttpOnly/SameSite","public HTTP rejected by client","Windows actual host collection and fresh report","public projection and history","authenticated binary SHA256"]},indent=2),encoding="utf-8")
    print("RELEASE_SMOKE_OK: isolated login, actual collector report, public projection, history and checksum; no services installed.")
finally:
    proc.terminate()
    try:proc.wait(timeout=12)
    except subprocess.TimeoutExpired:proc.kill();proc.wait(timeout=5)
    log.close()
    for name in ["admin-password.txt","smoke-key"]:
        path=data/name
        if path.exists():path.unlink()
