<script setup>
import { ref, onMounted, onUnmounted, watch } from "vue";
import { RefreshCw } from "lucide-vue-next";
import { api } from "../api";
import { revision } from "../state";
import { date, eventMessage } from "../format";
const logs = ref([]),
  servers = ref([]),
  level = ref(""),
  serverID = ref(""),
  page = ref(1),
  total = ref(0),
  error = ref(""),
  loading = ref(false);
let poll,
  busy = false;
async function refresh() {
  if (busy) return;
  busy = true;
  loading.value = true;
  try {
    const [l, s] = await Promise.all([
      api.logs({
        limit: 20,
        page: page.value,
        ...(level.value ? { level: level.value } : {}),
        ...(serverID.value ? { server_id: serverID.value } : {}),
      }),
      api.servers(),
    ]);
    logs.value = l.items.map((item) => ({
      ...item,
      message: eventMessage(item.message),
    }));
    total.value = l.total;
    servers.value = s;
    error.value = "";
  } catch (e) {
    error.value = e.message;
  } finally {
    busy = false;
    loading.value = false;
  }
}
onMounted(() => {
  refresh();
  poll = setInterval(refresh, 4000);
});
onUnmounted(() => clearInterval(poll));
watch(revision, refresh);
watch(page, refresh);
watch([level, serverID], () => {
  page.value = 1;
  refresh();
});
</script>
<template>
  <div class="log-tabs">
    <router-link to="/logs" class="active">系统事件</router-link
    ><router-link to="/operations">运维操作</router-link>
  </div>
  <div class="page-heading">
    <div>
      <span class="eyebrow">SYSTEM EVENTS</span>
      <h1>系统日志</h1>
      <p>追踪监控客户端连接、离线与资源异常，及时了解每一次变化。</p>
    </div>
    <el-button @click="refresh"><RefreshCw :size="16" />刷新日志</el-button>
  </div>
  <el-alert
    v-if="error"
    :title="error"
    type="error"
    :closable="false"
    class="page-error"
  />
  <section class="panel logs-panel">
    <div class="logs-toolbar">
      <el-select
        v-model="level"
        placeholder="全部等级"
        clearable
        style="width: 150px"
        ><el-option label="INFO · 信息" value="INFO" /><el-option
          label="WARN · 警告"
          value="WARN" /><el-option
          label="ERROR · 异常"
          value="ERROR" /></el-select
      ><el-select
        v-model="serverID"
        placeholder="全部服务器"
        clearable
        style="width: 210px"
        ><el-option
          v-for="s in servers"
          :key="s.id"
          :value="String(s.id)"
          :label="s.name" /></el-select
      ><span class="muted small">{{ total }} 条记录</span>
    </div>
    <el-table :data="logs" v-loading="loading" empty-text="暂无匹配的系统日志"
      ><el-table-column label="时间" width="185"
        ><template #default="{ row }"
          ><span class="mono small">{{ date(row.created_at) }}</span></template
        ></el-table-column
      ><el-table-column label="等级" width="100"
        ><template #default="{ row }"
          ><span class="log-badge" :class="row.level.toLowerCase()">{{
            row.level
          }}</span></template
        ></el-table-column
      ><el-table-column label="服务器" width="200"
        ><template #default="{ row }"
          ><router-link :to="`/servers/${row.server_id}`">{{
            servers.find((s) => s.id === row.server_id)?.name ||
            `#${row.server_id}`
          }}</router-link></template
        ></el-table-column
      ><el-table-column prop="message" label="事件" min-width="230"
    /></el-table>
    <div class="pagination">
      <el-pagination
        v-model:current-page="page"
        :page-size="20"
        :total="total"
        layout="prev, pager, next"
      />
    </div>
  </section>
</template>
