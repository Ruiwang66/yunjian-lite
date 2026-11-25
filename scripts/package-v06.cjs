// Package an explicit allowlist; never include tools, caches or runtime databases.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const root = path.resolve(__dirname, '..');
const roots = ['.dockerignore','.env.example','.gitignore','agent','bin','CHANGELOG.md','deploy','docker-compose.yml','Dockerfile','docs','go.mod','go.sum','internal','LICENSE','README.md','scripts','SECURITY.md','server','shared','tests','THIRD_PARTY_NOTICES.md','web/index.html','web/package.json','web/package-lock.json','web/vite.config.js','web/src','web/public','web/dist'];
const hash = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
const files = [];
function walk(relative) {
  const full = path.join(root, relative), stat = fs.lstatSync(full);
  if (stat.isSymbolicLink()) throw new Error('Unexpected symlink: '+relative);
  if (stat.isDirectory()) for (const name of fs.readdirSync(full)) walk(path.posix.join(relative,name));
  else files.push({path:relative.replaceAll('\\','/'),bytes:stat.size,sha256:hash(fs.readFileSync(full))});
}
for (const item of roots) walk(item);
const manifest = {version:'0.6.0',status:'local candidate; not deployed or published',baseline:'deployment-v05-20261106/release-v05.tar.gz',generated:new Date().toISOString(),files};
fs.writeFileSync(path.join(root,'deployment-manifest.json'),JSON.stringify(manifest,null,2)+'\n');
const archive=path.resolve(root,'../cloudease-lite-v0.6.0-candidate-20261111.tar.gz');
let result=spawnSync('tar',['-czf',archive,'-C',root,...roots,'deployment-manifest.json'],{encoding:'utf8'});
if(result.status!==0) throw new Error(result.stderr);
result=spawnSync('tar',['-tzf',archive],{encoding:'utf8'});
if(result.status!==0)throw new Error(result.stderr);
const entries=result.stdout.trim().split(/\r?\n/);
if(entries.some(f=>/(^|\/)(node_modules|\.runtime|\.tools|\.cache|\.git)(\/|$)|\.db(?:-wal|-shm)?$|(^|\/)\.env$|admin-password\.txt$/.test(f)))throw new Error('Private or local file in candidate');
const receipt={version:'0.6.0',archive,bytes:fs.statSync(archive).size,sha256:hash(fs.readFileSync(archive)),files:files.length,archiveEntries:entries.length,privatePathsExcluded:true};
fs.mkdirSync(path.join(root,'.runtime/v06'),{recursive:true});
fs.writeFileSync(path.join(root,'.runtime/v06/package-receipt.json'),JSON.stringify(receipt,null,2));
console.log(JSON.stringify(receipt,null,2));
