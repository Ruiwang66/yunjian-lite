<script setup>
import { ref, onMounted, onUnmounted, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ArrowLeft, KeyRound, Pencil, Trash2 } from "lucide-vue-next";
import { ElMessage, ElMessageBox } from "element-plus";
import { api, publicApi } from "../api";
const props = defineProps({ publicView: Boolean });
import { revision } from "../state";
import { bytes, date, ago, uptime, statusText, eventMessage } from "../format";
import MetricBar from "../components/MetricBar.vue";
import HistoryChart from "../components/HistoryChart.vue";
import TokenSetup from "../components/TokenSetup.vue";
import RemoveServer from "../components/RemoveServer.vue";
const route = useRoute(),
  router = useRouter(),
  server = ref(null),
  history = ref([]),
  logs = ref([]),
  hours = ref(6),
  error = ref(""),
  token = ref(""),
  tokenOpen = ref(false),
  removeOpen = ref(false),
  now = ref(Date.now());
let poll,
  clock,
  busy = false;
async function refresh() {
  if (busy || removeOpen.value) return;
  busy = true;
  try {
    const [s, h, l] = await Promise.all([
      (props.publicView ? publicApi : api).server(route.params.id),
      (props.publicView ? publicApi : api).history(
        route.params.id,
        hours.value,
      ),
      props.publicView
        ? Promise.resolve({ items: [] })
        : api.logs({ server_id: route.params.id, limit: 10 }),
    ]);
    server.value = s;
    history.value = h;
    logs.value = l.items;
    error.value = "";
  } catch (e) {
    error.value = e.message;
  } finally {
    busy = false;
  }
}
async function rename() {
  try {
    const { value } = await ElMessageBox.prompt(
      "为服务器设置一个易辨识的名称",
      "重命名",
      {
        inputValue: server.value.name,
        inputValidator: (value) =>
          (value?.trim().length > 0 && value.trim().length <= 80) ||
          "名称须为 1–80 个字符",
        confirmButtonText: "保存",
        cancelButtonText: "取消",
      },
    );
    await api.rename(server.value.id, value.trim());
    await refresh();
    ElMessage.success("名称已更新");
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message);
  }
}
async function rotate() {
  try {
    await ElMessageBox.confirm(
      "原连接密钥将立即失效。请使用新连接密钥重启监控客户端。",
      "重新生成连接密钥",
      { confirmButtonText: "生成新连接密钥", cancelButtonText: "取消" },
    );
    const result = await api.token(server.value.id);
    token.value = result.token;
    tokenOpen.value = true;
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message);
  }
}
function remove() {
  removeOpen.value = true;
}
onMounted(() => {
  refresh();
  poll = setInterval(refresh, 4000);
  clock = setInterval(() => (now.value = Date.now()), 1000);
});
onUnmounted(() => {
  clearInterval(poll);
  clearInterval(clock);
});
watch(revision, refresh);
watch(hours, refresh);
watch(() => route.params.id, refresh);
</script>
<template>
  <router-link :to="publicView ? '/' : '/servers'" class="back-link"
    ><ArrowLeft :size="15" />返回服务器</router-link
  ><el-alert
    v-if="error"
    :title="error"
    type="error"
    :closable="false"
    class="page-error"
  /><template v-if="server"
    ><div class="page-heading detail-heading">
      <div>
        <span class="eyebrow">SERVER DETAILS / #{{ server.id }}</span>
        <h1>
          {{ server.name
          }}<span class="status-pill" :class="server.status"
            ><i class="status-dot" :class="server.status"></i
            >{{ statusText[server.status] }}</span
          >
        </h1>
        <p>
          <template v-if="!publicView"
            >{{ server.ip || "等待监控客户端连接" }}
            <span class="separator">/</span></template
          >
          {{ server.platform || "未知系统" }} {{ server.platform_version }}
          <span class="separator">/</span> {{ server.arch || "—" }}
        </p>
      </div>
      <el-dropdown v-if="!publicView" trigger="click"
        ><el-button>管理服务器</el-button
        ><template #dropdown
          ><el-dropdown-menu
            ><el-dropdown-item @click="rename"
              ><Pencil :size="14" class="inline-icon" />重命名</el-dropdown-item
            ><el-dropdown-item @click="rotate"
              ><KeyRound :size="14" class="inline-icon" />重新生成
              连接密钥</el-dropdown-item
            ><el-dropdown-item divided @click="remove"
              ><Trash2
                :size="14"
                class="inline-icon"
              />删除连接密钥及服务器</el-dropdown-item
            ></el-dropdown-menu
          ></template
        ></el-dropdown
      >
    </div>
    <div v-if="server.status !== 'online'" class="offline-note">
      {{
        server.status === "pending"
          ? publicView
            ? "尚未接收到服务器数据。"
            : "尚未接收到监控客户端数据。请使用创建时的连接密钥启动监控客户端。"
          : "监控客户端当前离线。下面展示的是最后一次收到的数据，请检查监控客户端服务和网络连接。"
      }}
    </div>
    <div class="detail-metrics">
      <div class="panel">
        <MetricBar
          label="CPU 使用率"
          :value="server.latest?.cpu"
          :caption="`${server.cpu_cores || '—'} 逻辑核心`"
        />
      </div>
      <div class="panel">
        <MetricBar
          label="内存使用率"
          :value="server.latest?.memory"
          :caption="`${bytes(server.latest?.memory_used)} / ${bytes(server.latest?.memory_total)}`"
        />
      </div>
      <div class="panel">
        <MetricBar
          label="磁盘使用率"
          :value="server.latest?.disk"
          :caption="`${bytes(server.latest?.disk_used)} / ${bytes(server.latest?.disk_total)}`"
        />
      </div>
    </div>
    <section class="panel history-panel">
      <div class="section-heading">
        <div>
          <h2>资源使用趋势</h2>
          <p>CPU、内存与磁盘的历史采样</p>
        </div>
        <el-radio-group v-model="hours" size="small"
          ><el-radio-button :value="1">1 小时</el-radio-button
          ><el-radio-button :value="6">6 小时</el-radio-button
          ><el-radio-button :value="24"
            >24 小时</el-radio-button
          ></el-radio-group
        >
      </div>
      <HistoryChart :samples="history" />
    </section>
    <div class="detail-bottom-grid" :class="{ 'public-detail': publicView }">
      <section class="panel">
        <h2>系统信息</h2>
        <dl class="info-list">
          <div>
            <dt>操作系统</dt>
            <dd>{{ server.platform }} {{ server.platform_version || "—" }}</dd>
          </div>
          <div>
            <dt>运行时间</dt>
            <dd>{{ uptime(server.uptime) }}</dd>
          </div>
          <div>
            <dt>最近通信</dt>
            <dd>{{ ago(server.last_seen, now) }}</dd>
          </div>
          <div>
            <dt>累计下行 / 上行</dt>
            <dd>
              {{ bytes(server.latest?.network_rx) }} /
              {{ bytes(server.latest?.network_tx) }}
            </dd>
          </div>
          <div>
            <dt>实时下行 / 上行</dt>
            <dd>
              {{ bytes(server.latest?.network_rx_rate, true) }} /
              {{ bytes(server.latest?.network_tx_rate, true) }}
            </dd>
          </div>
          <div>
            <dt>{{ publicView ? "采集周期" : "添加时间" }}</dt>
            <dd>{{ publicView ? "4 秒" : date(server.created_at) }}</dd>
          </div>
        </dl>
      </section>
      <section v-if="!publicView" class="panel">
        <h2>服务器事件</h2>
        <div v-if="!logs.length" class="quiet-state">暂无事件</div>
        <div v-for="l in logs" :key="l.id" class="detail-log">
          <span class="log-badge" :class="l.level.toLowerCase()">{{
            l.level
          }}</span>
          <div>
            {{ eventMessage(l.message) }}<small>{{ date(l.created_at) }}</small>
          </div>
        </div>
      </section>
    </div></template
  >
  <div v-else-if="!error" class="empty-state">正在加载服务器…</div>
  <RemoveServer v-if="!publicView && server" v-model="removeOpen" :server="server" @removed="router.push('/servers')" />
  <el-dialog
    v-if="!publicView"
    v-model="tokenOpen"
    title="更新监控客户端连接"
    width="min(640px, 92vw)"
    @closed="token = ''"
    ><TokenSetup :token="token"
  /></el-dialog>
</template>
