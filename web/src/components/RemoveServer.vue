<script setup>
import { computed, ref, watch } from "vue";
import { Copy, Check } from "lucide-vue-next";
import { ElMessage } from "element-plus";
import { api } from "../api";
const props = defineProps({ modelValue: Boolean, server: Object });
const emit = defineEmits(["update:modelValue", "removed"]);
const visible = computed({
  get: () => props.modelValue,
  set: (value) => emit("update:modelValue", value),
});
const target = ref(null), fingerprint = ref(""), platform = ref("linux"),
  loading = ref(false), deleting = ref(false), deleted = ref(false),
  error = ref(""), copied = ref(false);
const shellQuote = (value) => "'" + String(value).replaceAll("'", "'\"'\"'") + "'";
const psQuote = (value) => "'" + String(value).replaceAll("'", "''") + "'";
const safeOrigin = computed(() => {
  const url = new URL(location.origin);
  return url.protocol === "https:" || (url.protocol === "http:" && ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname));
});
const command = computed(() => {
  if (!fingerprint.value || !safeOrigin.value) return "";
  const origin = location.origin;
  if (platform.value === "windows") {
    return [
      "& {",
      '  $script = Join-Path $env:TEMP ("cloudease-uninstall-" + [Guid]::NewGuid().ToString() + ".ps1")',
      "  try {",
      `    Invoke-WebRequest -UseBasicParsing -MaximumRedirection 0 ${psQuote(origin + "/uninstall-client.ps1")} -OutFile $script`,
      `    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $script -KeyFingerprint ${psQuote(fingerprint.value)}`,
      '    if ($LASTEXITCODE -ne 0) { throw "Monitoring client removal failed. Check the errors above." }',
      "  } finally {",
      "    Remove-Item -LiteralPath $script -Force -ErrorAction SilentlyContinue",
      "  }",
      "}",
    ].join("\n");
  }
  return [
    "(", "  set -e", "  script=$(mktemp)", `  trap 'rm -f "$script"' EXIT`,
    "  if command -v curl >/dev/null; then",
    `    curl -fsS --connect-timeout 15 --max-time 60 ${shellQuote(origin + "/uninstall-client.sh")} -o "$script"`,
    "  elif command -v wget >/dev/null; then",
    `    wget -q --timeout=30 --tries=2 --max-redirect=0 -O "$script" ${shellQuote(origin + "/uninstall-client.sh")}`,
    "  else", '    echo "请先安装 curl 或 wget。" >&2; exit 1', "  fi",
    `  bash "$script" --key-fingerprint ${shellQuote(fingerprint.value)}`, ")",
  ].join("\n");
});
watch(() => props.modelValue, async (open) => {
  if (!open) return;
  target.value = { id: props.server.id, name: props.server.name };
  fingerprint.value = ""; deleted.value = false; error.value = ""; copied.value = false;
  platform.value = props.server.os === "windows" ? "windows" : "linux";
  loading.value = true;
  const id = target.value.id;
  try {
    const data = await api.uninstall(id);
    if (props.modelValue && target.value?.id === id) fingerprint.value = data.key_fingerprint;
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
});
watch(platform, () => { copied.value = false; });
async function copy() {
  try {
    if (navigator.clipboard) await navigator.clipboard.writeText(command.value);
    else {
      const input = document.createElement("textarea");
      input.value = command.value; document.body.appendChild(input); input.select();
      const ok = document.execCommand("copy"); input.remove();
      if (!ok) throw new Error("copy failed");
    }
    copied.value = true;
    setTimeout(() => (copied.value = false), 2000);
  } catch {
    ElMessage.info("请手动选择并复制卸载命令");
  }
}
async function remove() {
  if (deleting.value || deleted.value || !fingerprint.value) return;
  deleting.value = true; error.value = "";
  try {
    await api.remove(target.value.id);
    deleted.value = true;
  } catch (e) { error.value = e.message; }
  finally { deleting.value = false; }
}
function closed() {
  if (deleted.value) emit("removed");
  fingerprint.value = ""; target.value = null;
}
</script>
<template>
  <el-dialog v-model="visible" title="删除连接密钥及服务器" width="min(640px, 92vw)"
    :close-on-click-modal="!deleting" :close-on-press-escape="!deleting"
    :show-close="!deleting" @closed="closed">
    <p><strong>{{ target?.name }}</strong></p>
    <el-alert v-if="deleted" title="服务器记录、指标和日志已删除，连接密钥已失效。客户端仍需在被监控服务器上执行下方命令卸载。"
      type="success" :closable="false" />
    <el-alert v-else title="确认删除将使连接密钥失效，并永久删除此服务器的指标和日志。客户端需要在被监控服务器上执行卸载命令。"
      type="warning" :closable="false" />
    <el-alert v-if="error" :title="error" type="error" :closable="false" class="page-error" />
    <div v-loading="loading">
      <p class="muted">先复制下方完整命令，在被监控服务器上执行：停止监控客户端、取消开机启动、删除客户端程序及本机密钥文件。然后确认删除控制台记录。</p>
      <el-radio-group v-model="platform">
        <el-radio-button value="linux">Linux 卸载</el-radio-button>
        <el-radio-button value="windows">Windows 卸载</el-radio-button>
      </el-radio-group>
      <p class="small muted">{{ platform === "linux" ? "使用 root 执行，适用于 systemd 自动安装。" : "使用管理员 PowerShell 执行，适用于自动安装创建的启动任务。" }}</p>
      <el-alert v-if="!safeOrigin" title="请通过 HTTPS 访问控制台后复制卸载命令；HTTP 仅支持本机回环测试。" type="error" :closable="false" />
      <template v-if="fingerprint && safeOrigin">
        <div class="code-block"><pre>{{ command }}</pre>
          <button @click="copy" :aria-label="copied ? '已复制卸载命令' : '复制完整卸载命令'">
            <Check v-if="copied" :size="16" /><Copy v-else :size="16" />
          </button>
        </div>
        <el-button type="primary" @click="copy">{{ copied ? "已复制" : "复制完整卸载命令" }}</el-button>
      </template>
      <p class="small muted">命令会核对本机连接密钥；不匹配时停止卸载。若重新生成密钥后未更新客户端，请先更新客户端配置。</p>
    </div>
    <template #footer>
      <el-button :disabled="deleting" @click="visible = false">{{ deleted ? "完成并返回" : "取消" }}</el-button>
      <el-button v-if="!deleted" type="danger" :loading="deleting" :disabled="loading || !fingerprint"
        @click="remove">确认删除密钥及记录</el-button>
    </template>
  </el-dialog>
</template>
