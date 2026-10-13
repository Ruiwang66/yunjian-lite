<script setup>
import { ref, watch, computed } from "vue";
import { ElMessageBox, ElMessage } from "element-plus";
import ManagementHeader from "../components/ManagementHeader.vue";
import { useManagement } from "../management";
const scope = useManagement(),
  services = ref([]),
  search = ref("");
const visible = computed(() =>
  services.value.filter((s) =>
    s.name.toLowerCase().includes(search.value.toLowerCase()),
  ),
);
const enabled = computed(
  () =>
    scope.info?.capabilities.enabled && scope.info?.server.status === "online",
);
async function refresh() {
  services.value = [];
  if (!enabled.value) return;
  try {
    services.value = await scope.execute("/service/list");
  } catch {}
}
watch(() => scope.info, refresh);
async function action(row, name) {
  try {
    await ElMessageBox.confirm(
      `确认${{ start: "启动", stop: "停止", restart: "重启" }[name]}服务 ${row.name}？`,
      "服务操作",
      { confirmButtonText: "确认执行", cancelButtonText: "取消" },
    );
    await scope.execute(
      "/service/action",
      { name: row.name, action: name },
      "POST",
    );
    ElMessage.success("服务操作完成");
    await refresh();
  } catch {}
}
</script>
<template>
  <ManagementHeader
    title="Linux 服务"
    description="查看 systemd 服务状态，只允许管理客户端白名单中的服务。"
    :scope="scope"
  />
  <div v-if="scope.info" class="panel management-panel">
    <div class="section-heading">
      <h2>服务列表</h2>
      <div class="management-selector">
        <el-input v-model="search" placeholder="搜索服务" clearable /><el-button
          :disabled="scope.busy || !enabled"
          @click="refresh"
          >刷新列表</el-button
        >
      </div>
    </div>
    <el-table :data="visible" empty-text="暂无服务，或系统未使用 systemd">
      <el-table-column prop="name" label="服务名称" min-width="200" />
      <el-table-column prop="state" label="状态" width="140" />
      <el-table-column prop="enabled" label="开机启动" width="160" />
      <el-table-column label="权限" width="130"
        ><template #default="{ row }">{{
          row.allowed ? "允许管理" : "仅查看"
        }}</template></el-table-column
      >
      <el-table-column label="操作" width="230"
        ><template #default="{ row }">
          <el-button
            v-for="a in ['start', 'stop', 'restart']"
            :key="a"
            link
            :disabled="
              scope.busy ||
              !scope.canOperate ||
              !row.allowed ||
              !scope.info.capabilities.service_write
            "
            @click="action(row, a)"
            >{{
              { start: "启动", stop: "停止", restart: "重启" }[a]
            }}</el-button
          >
        </template></el-table-column
      >
    </el-table>
    <p class="small muted">
      默认允许 nginx、mysql、docker、php-fpm、redis；SSH
      与内核相关服务禁止操作。
    </p>
  </div>
</template>
