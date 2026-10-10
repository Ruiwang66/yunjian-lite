const fs=require("node:fs"), vm=require("node:vm"), assert=require("node:assert/strict");
fs.mkdirSync(".runtime",{recursive:true});
const source=fs.readFileSync("web/src/components/TokenSetup.vue","utf8").split("<script setup>")[1].split("</script>")[0].replace(/^import .*;\n/gm,"");
const remove=fs.readFileSync("web/src/components/RemoveServer.vue","utf8").split("<script setup>")[1].split("</script>")[0].replace(/^import .*;\n/gm,"");
const key="A".repeat(43);
for(const origin of ["https://monitor.example","http://127.0.0.1:5050","http://[::1]:5050","http://localhost:5050","http://monitor.example:5050"]){
 for(const platform of ["linux","windows"]){
  const ctx={URL,computed:fn=>({get value(){return fn();}}),ref:value=>({value}),watch:()=>{},defineProps:()=>({token:key}),defineEmits:()=>()=>{},location:{origin}};
  vm.createContext(ctx);vm.runInContext(source+'\nplatform.value="'+platform+'";globalThis.generated=command.value;',ctx);
  const safe=!origin.includes("http://monitor.example");
  assert.equal(Boolean(ctx.generated),safe);
  if(safe){
   assert.ok(ctx.generated.includes("/install-client."));
   assert.ok(!ctx.generated.includes(key),"key leaked into shell history");
   assert.ok(!/--key |--token |-Key |-Token /.test(ctx.generated),"key command argument retained");
   assert.ok(ctx.generated.includes("MaximumRedirection 0")||ctx.generated.includes("--max-redirect=0"),"redirect guard missing");
   if(origin==="https://monitor.example")fs.writeFileSync(".runtime/generated-"+platform+(platform==="linux"?".sh":".ps1"),ctx.generated);
  }
  const rm={URL,computed:fn=>({get value(){return fn();}}),ref:value=>({value}),watch:()=>{},defineProps:()=>({}),defineEmits:()=>()=>{},location:{origin}};
  vm.createContext(rm);vm.runInContext(remove+'\nfingerprint.value="a".repeat(64);platform.value="'+platform+'";globalThis.generated=command.value;',rm);
  assert.equal(Boolean(rm.generated),safe,"uninstall must not execute public HTTP");
 }
}
console.log("INSTALL_COMMAND_SECURITY_OK: no key in commands; HTTPS/loopback only; install and uninstall.");
