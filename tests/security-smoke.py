"""Run release binaries against an isolated loopback fixture; no installed services or real database."""
import os, json, time, socket, subprocess, urllib.request, urllib.error, pathlib, tempfile, hashlib, http.server, threading, datetime, sqlite3
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
    assert status==200 and result["version"]=="0.2.0"
    first=(data/"admin-password.txt").read_text().split("password: ")[1].strip()
    status,headers,result=call("/api/auth/login",{"username":"admin","password":first})
    assert result["user"]["role"]=="admin"
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
    status,headers,system=call("/api/system/info?server_id="+str(server_id))
    assert status==200 and system["system"]["cpu_model"] and system["system"]["kernel_version"],system
    status,headers,management=call("/api/management/info?server_id="+str(server_id))
    assert status==200 and management["capabilities"]["version"]=="0.2.0"
    if os.name=="nt":assert management["capabilities"]["enabled"] is False
    # A loopback v0.1 control-plane fixture rejects extra fields just like decodeJSON.
    legacy_reports=[]
    legacy_fields={"sent_at","nonce","name","ip","os","platform","platform_version","arch","cpu_cores","cpu","memory","memory_total","memory_used","disk","disk_total","disk_used","network_rx","network_tx","network_rx_rate","network_tx_rate","uptime"}
    expected_key=key_path.read_text().strip()
    class Legacy(http.server.BaseHTTPRequestHandler):
        def log_message(self,*args):pass
        def reply(self,status,body):
            raw=json.dumps(body).encode();self.send_response(status);self.send_header("Content-Type","application/json");self.send_header("Content-Length",str(len(raw)));self.end_headers();self.wfile.write(raw)
        def do_GET(self):
            if self.path!="/api/agent/check" or self.headers.get("Authorization")!="Bearer "+expected_key:self.reply(401,{});return
            self.reply(200,{"ok":True})
        def do_POST(self):
            report=json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            if self.path!="/api/agent/report" or self.headers.get("Authorization")!="Bearer "+expected_key or set(report)-legacy_fields:self.reply(400,{"error":"unknown fields"});return
            legacy_reports.append(report);self.reply(200,{"ok":True})
    legacy=http.server.ThreadingHTTPServer(("127.0.0.1",0),Legacy)
    thread=threading.Thread(target=legacy.serve_forever,daemon=True);thread.start()
    try:
        run=subprocess.run([str(agent_path),"--server","http://127.0.0.1:"+str(legacy.server_port),"--token-file",str(key_path),"--once"],capture_output=True,text=True,timeout=25,creationflags=flags)
        assert run.returncode==0 and len(legacy_reports)==1,run.stderr
        assert "system" not in legacy_reports[0] and "management" not in legacy_reports[0]
    finally:legacy.shutdown();legacy.server_close();thread.join(timeout=3)
    # Check real release-binary scheduling separately from the once-only collector.
    status,headers,new=call("/api/servers",{"name":"four-second cadence fixture"})
    cadence_id=new["server"]["id"];cadence_key=data/"cadence-key";cadence_key.write_text(new["token"],encoding="utf-8")
    if os.name!="nt":cadence_key.chmod(0o600)
    agent_log=(folder/"cadence.log").open("wb")
    cadence=subprocess.Popen([str(agent_path),"--server",origin,"--token-file",str(cadence_key)],stdout=agent_log,stderr=subprocess.STDOUT,creationflags=flags)
    try:
        deadline=time.monotonic()+16;times=[]
        with sqlite3.connect("file:"+(data/"cloudease.db").as_posix()+"?mode=ro",uri=True) as conn:
            while time.monotonic()<deadline:
                rows=conn.execute("SELECT created_at FROM metrics WHERE server_id=? ORDER BY id",(cadence_id,)).fetchall()
                if len(rows)>=3:times=[datetime.datetime.fromisoformat(row[0]) for row in rows[:3]];break
                time.sleep(.3)
        assert len(times)==3,"fewer than three actual host samples"
        intervals=[(times[i]-times[i-1]).total_seconds() for i in range(1,3)]
        assert all(3.0<=value<=5.5 for value in intervals),intervals
        print("V02_SAMPLE_INTERVALS_SECONDS:",*[round(v,3) for v in intervals])
    finally:
        cadence.terminate()
        try:cadence.wait(timeout=8)
        except subprocess.TimeoutExpired:cadence.kill();cadence.wait(timeout=3)
        agent_log.close();cadence_key.unlink(missing_ok=True)
    for route in ["/docker","/services","/files","/security","/system","/operations","/settings"]:
        with urllib.request.urlopen(origin+route,timeout=5) as response:assert response.status==200 and b'id="app"' in response.read()

    (folder/"result.json").write_text(json.dumps({"status":"passed","scope":"isolated loopback release binaries","server_binary":str(server_path),"agent_binary":str(agent_path),"checks":["bootstrap and login","HttpOnly/SameSite","public HTTP rejected by client","Windows actual host collection and fresh report","public projection and history","authenticated binary SHA256","v0.2 role and actual system metadata","v0.2 client to v0.1 strict fixture","actual four-second cadence","new SPA route serving"]},indent=2),encoding="utf-8")
    print("RELEASE_SMOKE_OK: v0.2 login, real collector/system details, four-second cadence, v0.1 compatibility, private/public APIs, SPA routes and checksums; no services installed.")
finally:
    proc.terminate()
    try:proc.wait(timeout=12)
    except subprocess.TimeoutExpired:proc.kill();proc.wait(timeout=5)
    log.close()
    for name in ["admin-password.txt","smoke-key"]:
        path=data/name
        if path.exists():path.unlink()
