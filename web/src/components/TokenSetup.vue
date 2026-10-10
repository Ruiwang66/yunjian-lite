<script setup>
import { computed, ref } from "vue";
import { Copy, Download, Check } from "lucide-vue-next";
import { ElMessage } from "element-plus";
const props = defineProps({ token: String }),
  platform = ref("linux"),
  copied = ref(false);
const shellQuote = (value) =>
  "'" + String(value).replaceAll("'", "'\"'\"'") + "'";
const psQuote = (value) => "'" + String(value).replaceAll("'", "''") + "'";
const safeOrigin = computed(() => {
  const url = new URL(location.origin);
  return url.protocol === "https:" || (url.protocol === "http:" && ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname));
});
const command = computed(() => {
  if (!safeOrigin.value) return "";
  const origin = location.origin;
  if (platform.value === "windows") {
    return [
      "& {",
      '  $script = Join-Path $env:TEMP ("cloudease-install-" + [Guid]::NewGuid().ToString() + ".ps1")',
      "  try {",
      `    Invoke-WebRequest -UseBasicParsing -MaximumRedirection 0 ${psQuote(origin + "/install-client.ps1")} -OutFile $script`,
      `    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $script -Server ${psQuote(origin)}`,
      '    if ($LASTEXITCODE -ne 0) { throw "Monitoring client installation failed. Check the errors above." }',
      "  } finally {",
      "    Remove-Item -LiteralPath $script -Force -ErrorAction SilentlyContinue",
      "  }",
      "}",
    ].join("\n");
  }
  return [
    "(",
    "  set -e",
    "  script=$(mktemp)",
    `  trap 'rm -f "$script"' EXIT`,
    "  if command -v curl >/dev/null; then",
    `    curl -fsS --connect-timeout 15 --max-time 60 ${shellQuote(origin + "/install-client.sh")} -o "$script"`,
    "  elif command -v wget >/dev/null; then",
    `    wget -q --timeout=30 --tries=2 --max-redirect=0 -O "$script" ${shellQuote(origin + "/install-client.sh")}`,
    "  else",
    '    echo "请先安装 curl 或 wget。" >&2; exit 1',
    "  fi",
    `  bash "$script" --server ${shellQuote(origin)}`,
    ")",
  ].join("\n");
});
async function copy(value = command.value, isKey = false) {
  try {
    if (navigator.clipboard) await navigator.clipboard.writeText(value);
    else {
      const input = document.createElement("textarea");
      input.value = value;
      document.body.appendChild(input);
      input.select();
      const ok = document.execCommand("copy");
      input.remove();
      if (!ok) throw new Error("copy failed");
    }
    if (isKey) ElMessage.success("连接密钥已复制");
    else { copied.value = true; setTimeout(() => (copied.value = false), 2000); }
  } catch {
    ElMessage.info("请手动选择并复制安装命令");
  }
}
</script>
<template>
  <div class="token-setup">
    <el-alert
      title="连接密钥仅显示一次，请妥善保管，勿公开分享。安装时会隐藏读取密钥。"
      type="info"
      :closable="false"
    />
    <p class="muted">
      先复制安装命令到被监控服务器执行，再复制下方连接密钥，在安装提示中粘贴。安装会保存密钥并配置开机启动。
    </p>
    <el-alert v-if="!safeOrigin" title="请通过 HTTPS 访问控制台后安装或卸载客户端；HTTP 仅支持本机回环测试。" type="error" :closable="false" />
    <div v-if="safeOrigin" class="download-row">
      <el-input :model-value="props.token" type="password" show-password readonly autocomplete="off" aria-label="连接密钥" />
      <el-button @click="copy(props.token, true)">复制连接密钥</el-button>
    </div>
    <el-radio-group v-model="platform">
      <el-radio-button value="linux">Linux 自动安装</el-radio-button>
      <el-radio-button value="windows">Windows 自动安装</el-radio-button>
    </el-radio-group>
    <p class="small muted">
      {{
        platform === "linux"
          ? "使用 root 执行；自动识别 amd64 / arm64，需要 systemd 和 curl 或 wget。"
          : "使用管理员 PowerShell 执行；支持 Windows x64，自动创建启动任务。"
      }}
    </p>
    <div v-if="safeOrigin" class="code-block">
      <pre>{{ command }}</pre>
      <button
        @click="copy()"
        :aria-label="copied ? '已复制安装命令' : '复制完整安装命令'"
      >
        <Check v-if="copied" :size="16" /><Copy v-else :size="16" />
      </button>
    </div>
    <el-button v-if="safeOrigin" type="primary" @click="copy()">{{
      copied ? "已复制" : "复制完整安装命令"
    }}</el-button>
    <p class="small muted">也可以手动下载：</p>
    <div class="download-row">
      <a
        :href="`/api/downloads/${platform === 'windows' ? 'cloud-agent.exe' : 'cloud-agent-linux-amd64'}`"
        class="button-secondary"
        download
        ><Download :size="16" />{{
          platform === "windows" ? "Windows x64 监控客户端" : "Linux amd64 监控客户端"
        }}</a
      >
      <a
        v-if="platform === 'linux'"
        href="/api/downloads/cloud-agent-linux-arm64"
        download
        class="text-link"
        >Linux arm64 监控客户端</a
      >
    </div>
    <p v-if="platform === 'linux'" class="small muted">
      Linux 查看日志：journalctl -u cloud-agent -n 20 --no-pager
    </p>
  </div>
</template>
