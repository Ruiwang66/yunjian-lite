const fs = require("node:fs"), vm = require("node:vm");
fs.mkdirSync(".runtime",{recursive:true});
const source = fs.readFileSync("web/src/components/RemoveServer.vue","utf8").split("<script setup>")[1].split("</script>")[0].replace(/^import .*;\n/gm,"");
for (const platform of ["linux","windows"]) {
  const ctx={URL,computed:fn=>({get value(){return fn();}}),ref:value=>({value}),watch:()=>{},defineProps:()=>({}),defineEmits:()=>()=>{},location:{origin:"https://monitor.example"}};
  vm.createContext(ctx);
  vm.runInContext(source+'\nfingerprint.value = "a".repeat(64); platform.value = "'+platform+'"; globalThis.generated = command.value;',ctx);
  if (!ctx.generated.includes("/uninstall-client.") || !ctx.generated.includes("a".repeat(64))) throw Error("missing matching removal command");
  fs.writeFileSync(".runtime/generated-remove-"+platform+(platform==="linux"?".sh":".ps1"),ctx.generated);
}
console.log("Generated uninstall commands from the actual Vue component.");
