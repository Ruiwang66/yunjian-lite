<script setup>
import { computed, ref } from "vue";
import { Copy, Check } from "lucide-vue-next";
import { ElMessage } from "element-plus";
const props = defineProps({ server: Object, enabled: Boolean });
const copied = ref(false);
const linux = computed(() => props.server?.os === "linux");
const safeOrigin = computed(() => {
  const url = new URL(location.origin);
  return (
    url.protocol === "https:" ||
    (url.protocol === "http:" &&
      ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname))
  );
});
const quote = (value) => "'" + String(value).replaceAll("'", "'\"'\"'") + "'";
const command = computed(() =>
  safeOrigin.value
    ? [
        "(",
        "  set -e",
        "  script=$(mktemp)",
        `  trap 'rm -f "$script"' EXIT`,
        "  if command -v curl >/dev/null; then",
        `    curl -fsS --connect-timeout 15 --max-time 60 ${quote(location.origin + "/enable-security.sh")} -o "$script"`,
        "  elif command -v wget >/dev/null; then",
        `    wget -q --timeout=30 --tries=2 --max-redirect=0 -O "$script" ${quote(location.origin + "/enable-security.sh")}`,
        "  else",
        '    echo "请先安装 curl 或 wget。" >&2; exit 1',
        "  fi",
        '  bash "$script"',
        ")",
      ].join("\n")
    : "",
);
async function copy() {
  try {
    await navigator.clipboard.writeText(command.value);
    copied.value = true;
    ElMessage.success("启用命令已复制，请在当前选中服务器上执行");
  } catch {
    ElMessage.info("请手动选中并复制下方完整命令");
  }
}
</script>
<template>
  <div class="security-setup">
    <h3>{{ server?.name }}</h3>
    <el-alert
      v-if="enabled"
      title="此服务器已开启安全检测，关闭窗口后点击“开始只读检测”即可生成结果。"
      type="success"
      :closable="false"
    />
    <el-alert
      v-else-if="!linux"
      :title="
        server?.os === 'windows'
          ? 'Windows 暂不支持安全检测，原有监控功能正常使用。'
          : '尚未识别客户端系统，请先安装并连接 v0.4 Linux 客户端。'
      "
      type="info"
      :closable="false"
    />
    <template v-if="linux">
      <p>
        在这台服务器的终端中使用 root
        执行下方命令。网页只提供指引，不会自动修改其他机器。
      </p>
      <ol>
        <li>
          需要已运行的 v0.4 Linux
          客户端，并已明确启用运维管理；Windows、普通账户和容器模式不会自动提升权限。
        </li>
        <li>
          命令会备份服务配置，保留原密钥、启动参数和服务限制，仅追加只读安全检测开关并重启客户端。启动失败时自动撤销本次配置。
        </li>
        <li>
          执行成功后点击“刷新启用状态”，再点击“开始只读检测”。检测按需执行，不会自动修复或封禁。
        </li>
      </ol>
      <p class="small muted">
        若尚未授权运维管理，请在 Linux 自动安装中同时勾选“启用运维管理”和“启用
        v0.4
        只读安全检测”。不要仅为开启检测重新生成连接密钥。日志读取仍需主机本地白名单。
      </p>
      <el-alert
        v-if="!safeOrigin"
        title="请通过 HTTPS 打开控制台后复制启用命令。"
        type="warning"
        :closable="false"
      />
      <template v-else>
        <div class="code-block">
          <pre
            tabindex="0"
            aria-label="完整安全检测启用命令，可使用方向键滚动"
            >{{ command }}</pre>
          <button
            @click="copy"
            :aria-label="copied ? '已复制启用命令' : '复制启用命令'"
          >
            <Check v-if="copied" :size="16" /><Copy v-else :size="16" />
          </button>
        </div>
        <el-button type="primary" @click="copy">{{
          copied ? "已复制启用命令" : "复制启用命令"
        }}</el-button>
      </template>
    </template>
    <p class="small muted">
      <router-link :to="`/servers/${server?.id}`" class="text-link"
        >查看此服务器详情</router-link
      >
    </p>
  </div>
</template>
<style scoped>
.security-setup p,
.security-setup li {
  line-height: 1.8;
}
.security-setup li + li {
  margin-top: 10px;
}
.security-setup ol {
  padding-left: 22px;
}
.security-setup .code-block {
  margin: 20px 0;
}
</style>
