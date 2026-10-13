<script setup>
import { ref, watch, computed } from "vue";
import { ElMessageBox, ElMessage } from "element-plus";
import ManagementHeader from "../components/ManagementHeader.vue";
import { useManagement } from "../management";
const scope = useManagement(),
  containers = ref([]),
  logs = ref(""),
  logsOpen = ref(false);
const enabled = computed(
  () =>
    scope.info?.capabilities.enabled && scope.info?.server.status === "online",
);
async function refresh() {
  containers.value = [];
  if (!enabled.value) return;
  try {
    containers.value = await scope.execute("/docker/container");
  } catch {}
}
watch(() => scope.info, refresh);
async function action(row, name) {
  try {
    await ElMessageBox.confirm(
      `确认${{ start: "启动", stop: "停止", restart: "重启" }[name]}容器 ${row.name}？`,
      "容器操作",
      { confirmButtonText: "确认执行", cancelButtonText: "取消" },
    );
    await scope.execute("/docker/container/" + name, { id: row.id }, "POST");
    ElMessage.success("容器操作完成");
    await scope.loadInfo();
  } catch {}
}
async function showLogs(row) {
  try {
    const data = await scope.execute("/docker/container/logs", { id: row.id });
    logs.value = data.output;
    logsOpen.value = true;
  } catch {}
}
</script>
<template>
  <ManagementHeader
    title="Docker"
    description="查看 Docker 状态与容器，管理常用生命周期操作。"
    :scope="scope"
  />
  <template v-if="scope.info">
    <div class="stats-grid management-stats">
      <div class="stat-card">
        <div class="stat-label">Docker 状态</div>
        <div class="stat-value compact">
          {{ scope.info.capabilities.docker?.available ? "运行中" : "不可用" }}
        </div>
        <div class="stat-note">
          版本 {{ scope.info.capabilities.docker?.version || "—" }}
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-label">容器</div>
        <div class="stat-value">
          {{ scope.info.capabilities.docker?.containers || 0 }}
        </div>
      </div>
      <div class="stat-card">
        <div class="stat-label">镜像</div>
        <div class="stat-value">
          {{ scope.info.capabilities.docker?.images || 0 }}
        </div>
      </div>
    </div>
    <div class="panel management-panel">
      <div class="section-heading">
        <h2>容器列表</h2>
        <el-button :disabled="scope.busy || !enabled" @click="refresh"
          >刷新列表</el-button
        >
      </div>
      <el-table
        :data="containers"
        empty-text="暂无容器，或 Docker 未安装 / 权限不足"
      >
        <el-table-column prop="name" label="名称" min-width="170" />
        <el-table-column prop="state" label="状态" width="110" />
        <el-table-column prop="ports" label="端口" min-width="200" />
        <el-table-column prop="created_at" label="创建时间" min-width="180" />
        <el-table-column label="操作" width="270"
          ><template #default="{ row }">
            <el-button
              link
              :disabled="
                scope.busy ||
                !scope.canOperate ||
                !scope.info.capabilities.docker_write
              "
              @click="action(row, 'start')"
              >启动</el-button
            >
            <el-button
              link
              :disabled="
                scope.busy ||
                !scope.canOperate ||
                !scope.info.capabilities.docker_write
              "
              @click="action(row, 'stop')"
              >停止</el-button
            >
            <el-button
              link
              :disabled="
                scope.busy ||
                !scope.canOperate ||
                !scope.info.capabilities.docker_write
              "
              @click="action(row, 'restart')"
              >重启</el-button
            >
            <el-button link :disabled="scope.busy" @click="showLogs(row)"
              >日志</el-button
            >
          </template></el-table-column
        >
      </el-table>
    </div>
  </template>
  <el-dialog
    v-model="logsOpen"
    title="容器日志 · 最近 100 行"
    width="min(900px, 94vw)"
    @closed="logs = ''"
  >
    <pre class="operation-output">{{ logs || "暂无日志" }}</pre>
    <p class="small muted">
      输出最多 16 KiB，常见凭据字段自动隐藏。
    </p></el-dialog
  >
</template>
