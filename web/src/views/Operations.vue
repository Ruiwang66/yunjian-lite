<script setup>
import { ref, onMounted, onUnmounted, watch } from "vue";
import { request } from "../api";
import { date } from "../format";
const items = ref([]),
  total = ref(0),
  page = ref(1),
  status = ref(""),
  error = ref(""),
  loading = ref(false);
let timer;
const labels = {
  requested: "已请求",
  queued: "等待执行",
  running: "执行中",
  success: "成功",
  failed: "失败",
  expired: "已过期",
  cancelled: "已取消",
};
const actions = {
  "POST /api/auth/login": "登录控制台",
  "POST /api/auth/logout": "退出登录",
  "POST /api/auth/password": "修改账户密码",
  "POST /api/servers": "添加服务器",
  "PATCH /api/servers/:id": "重命名服务器",
  "DELETE /api/servers/:id": "删除服务器与连接密钥",
  "POST /api/servers/:id/token": "更新连接密钥",
  "GET /api/docker/info": "读取 Docker 状态",
  "GET /api/system/info": "读取系统信息",
  "GET /api/users": "查看用户",
  "POST /api/users": "创建用户",
  "PATCH /api/users/:id": "更新用户权限或密码",
  "DELETE /api/users/:id": "删除用户",
  "GET /api/roles": "读取角色",
  "docker.list": "读取容器",
  "docker.logs": "查看容器日志",
  "docker.start": "启动容器",
  "docker.stop": "停止容器",
  "docker.restart": "重启容器",
  "service.list": "读取服务",
  "service.start": "启动服务",
  "service.stop": "停止服务",
  "service.restart": "重启服务",
  "file.list": "浏览目录",
  "file.read": "查看文件",
  "file.download": "下载文件",
  "file.upload": "上传文件",
  "command.disk": "查看磁盘",
  "command.memory": "查看内存",
  "command.network": "查看网络",
  "command.journal": "查看系统日志",
};
async function refresh() {
  if (loading.value) return;
  loading.value = true;
  try {
    const r = await request(
      "/operation-logs?" +
        new URLSearchParams({ page: page.value, status: status.value }),
    );
    items.value = r.items;
    total.value = r.total;
    error.value = "";
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}
watch(page, refresh);
watch(status, () => {
  page.value = 1;
  refresh();
});
onMounted(() => {
  refresh();
  timer = setInterval(refresh, 5000);
});
onUnmounted(() => clearInterval(timer));
</script>
<template>
  <div class="page-heading">
    <div>
      <span class="eyebrow">OPERATION LOG</span>
      <h1>运维日志</h1>
      <p>追踪用户、来源和操作状态，日志保留 90 天。</p>
    </div>
    <el-button :loading="loading" @click="refresh">刷新</el-button>
  </div>
  <div class="log-tabs">
    <router-link to="/logs">系统事件</router-link
    ><router-link to="/operations" class="active">运维操作</router-link>
  </div>
  <el-alert
    v-if="error"
    :title="error"
    type="error"
    :closable="false"
    class="page-error"
  />
  <div class="panel management-panel">
    <el-select v-model="status" placeholder="所有状态" clearable
      ><el-option
        v-for="(label, value) in labels"
        :key="value"
        :value="value"
        :label="label"
    /></el-select>
    <el-table :data="items" empty-text="暂无运维记录"
      ><el-table-column label="时间" min-width="180"
        ><template #default="{ row }">{{
          date(row.created_at)
        }}</template></el-table-column
      ><el-table-column prop="user" label="用户" width="120" /><el-table-column
        prop="ip"
        label="来源 IP"
        min-width="150"
      /><el-table-column label="操作" min-width="210"
        ><template #default="{ row }">{{
          actions[row.action] || row.action
        }}</template></el-table-column
      ><el-table-column
        prop="server_id"
        label="服务器"
        width="100"
      /><el-table-column
        prop="target"
        label="目标"
        min-width="220"
      /><el-table-column label="状态" width="120"
        ><template #default="{ row }"
          ><span class="operation-status" :class="row.status">{{
            labels[row.status] || row.status
          }}</span></template
        ></el-table-column
      ></el-table
    >
    <el-pagination
      v-model:current-page="page"
      :page-size="50"
      :total="total"
      layout="prev, pager, next, total"
    />
    <p class="small muted">
      操作过期时，执行状态可能未知。请检查服务器实际状态后再决定是否重试。
    </p>
  </div>
</template>
