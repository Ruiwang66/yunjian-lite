<script setup>
import { ref, computed, watch } from "vue";
import { ElMessageBox } from "element-plus";
import ManagementHeader from "../components/ManagementHeader.vue";
import { useManagement } from "../management";
const scope = useManagement(),
  output = ref("");
const commands = [
  { name: "disk", label: "查看磁盘", command: "df -h" },
  { name: "memory", label: "查看内存", command: "free -m" },
  { name: "network", label: "查看网络", command: "ip addr" },
  { name: "journal", label: "查看日志", command: "journalctl（最近 100 行）" },
];
const allowed = computed(
  () =>
    scope.canOperate &&
    scope.info?.capabilities.enabled &&
    scope.info?.server.status === "online",
);
watch(
  () => scope.selected,
  () => {
    output.value = "";
  },
);
async function execute(item) {
  try {
    await ElMessageBox.confirm(
      `在选中的服务器执行「${item.label}」？`,
      "执行预定义命令",
      { confirmButtonText: "执行", cancelButtonText: "取消" },
    );
    const result = await scope.execute(
      "/commands/execute",
      { name: item.name },
      "POST",
    );
    output.value = result.output;
  } catch {}
}
</script>
<template>
  <ManagementHeader
    title="命令与安全"
    description="使用预定义的只读运维命令，每次执行都会记录到运维日志。"
    :scope="scope"
  />
  <div class="command-grid">
    <div v-for="item in commands" :key="item.name" class="panel">
      <h2>{{ item.label }}</h2>
      <p>
        <code>{{ item.command }}</code>
      </p>
      <el-button :disabled="!allowed || scope.busy" @click="execute(item)"
        >执行命令</el-button
      >
    </div>
  </div>
  <div class="panel management-panel">
    <h2>命令结果</h2>
    <pre class="operation-output">{{
      output || "选择服务器，再执行上方命令。"
    }}</pre>
    <p class="small muted">
      输出最多 16 KiB，常见凭据字段自动隐藏。只读用户无法执行命令。
    </p>
  </div>
  <div class="panel management-panel" v-if="scope.info">
    <h2>客户端权限</h2>
    <dl class="system-grid">
      <div>
        <dt>运维管理</dt>
        <dd>{{ scope.info.capabilities.enabled ? "已启用" : "未启用" }}</dd>
      </div>
      <div>
        <dt>客户端权限</dt>
        <dd>
          {{
            scope.info.capabilities.privileged
              ? scope.info.server.os === "windows"
                ? "管理员"
                : "root"
              : "普通用户"
          }}
        </dd>
      </div>
      <div>
        <dt>Docker 操作</dt>
        <dd>{{ scope.info.capabilities.docker_write ? "允许" : "未授予" }}</dd>
      </div>
      <div>
        <dt>服务操作</dt>
        <dd>
          {{
            scope.info.capabilities.service_write ? "允许白名单服务" : "未授予"
          }}
        </dd>
      </div>
    </dl>
  </div>
</template>
