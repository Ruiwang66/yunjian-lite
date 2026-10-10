<script setup>
import { computed, ref, onMounted, onUnmounted, watch } from "vue";
import { useRoute } from "vue-router";
import {
  Plus,
  Search,
  ArrowUpRight,
  Server,
  CircleCheck,
  RefreshCw,
  ArrowDown,
  ArrowUp,
  Activity,
  Clock3,
} from "lucide-vue-next";
import { api, publicApi } from "../api";
const props = defineProps({ publicView: Boolean });
import { revision } from "../state";
import { ago, percent, bytes, statusText, date, eventMessage } from "../format";
import MetricBar from "../components/MetricBar.vue";
import AddServer from "../components/AddServer.vue";
const route = useRoute(),
  servers = ref([]),
  logs = ref([]),
  loading = ref(true),
  error = ref(""),
  addOpen = ref(false),
  search = ref(""),
  filter = ref("all"),
  now = ref(Date.now()),
  loaded = ref(null);
const online = computed(() =>
  servers.value.filter((s) => s.status === "online"),
);
const offline = computed(() =>
  servers.value.filter((s) => s.status === "offline"),
);
const averageCPU = computed(() =>
  online.value.length
    ? online.value.reduce((sum, s) => sum + (s.latest?.cpu || 0), 0) /
      online.value.length
    : null,
);
const network = computed(() =>
  online.value.reduce(
    (sum, s) =>
      sum + (s.latest?.network_rx_rate || 0) + (s.latest?.network_tx_rate || 0),
    0,
  ),
);
const visible = computed(() =>
  servers.value.filter(
    (s) =>
      (filter.value === "all" || s.status === filter.value) &&
      `${s.name} ${s.ip} ${s.platform}`
        .toLowerCase()
        .includes(search.value.toLowerCase()),
  ),
);
const title = computed(() =>
  route.path === "/servers" ? "服务器" : "基础设施概览",
);
let poll,
  clock,
  busy = false;
async function refresh() {
  if (busy) return;
  busy = true;
  try {
    const [items, events] = await Promise.all([
      props.publicView ? publicApi.servers() : api.servers(),
      props.publicView
        ? Promise.resolve({ items: [] })
        : api.logs({ limit: 5 }),
    ]);
    servers.value = items;
    logs.value = events.items;
    error.value = "";
    loaded.value = Date.now();
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
    busy = false;
  }
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
</script>
<template>
  <div class="page-heading">
    <div>
      <span class="eyebrow">INFRASTRUCTURE OVERVIEW</span>
      <h1>{{ title }}</h1>
      <p>关注关键指标，让每一台服务器保持最佳状态。</p>
    </div>
    <el-button
      v-if="!publicView"
      type="primary"
      size="large"
      @click="addOpen = true"
      ><Plus :size="17" />添加服务器</el-button
    >
  </div>
  <el-alert
    v-if="error"
    :title="error"
    type="error"
    :closable="false"
    show-icon
    class="page-error"
  />
  <div class="stats-grid">
    <div class="stat-card">
      <div class="stat-label">服务器总数<Server :size="17" /></div>
      <div class="stat-value">{{ servers.length }}<span>台</span></div>
      <div class="stat-note">你的基础设施规模</div>
    </div>
    <div class="stat-card">
      <div class="stat-label">在线服务器<CircleCheck :size="17" /></div>
      <div class="stat-value">
        {{ online.length }}<span>/ {{ servers.length }}</span>
      </div>
      <div class="stat-note">
        <i class="status-dot online"></i
        >{{
          offline.length ? `${offline.length} 台离线，请关注` : "连接状态正常"
        }}
      </div>
    </div>
    <div class="stat-card">
      <div class="stat-label">平均 CPU 使用率<Activity :size="17" /></div>
      <div class="stat-value">
        {{ percent(averageCPU) }}<span v-if="averageCPU !== null">%</span>
      </div>
      <div class="stat-note">在线服务器实时平均值</div>
    </div>
    <div class="stat-card">
      <div class="stat-label">网络吞吐<ArrowUpRight :size="17" /></div>
      <div class="stat-value stat-rate">{{ bytes(network, true) }}</div>
      <div class="stat-note">在线服务器上下行总速率</div>
    </div>
  </div>
  <section class="fleet-section">
    <div class="section-heading">
      <div>
        <h2>
          服务器资源<span class="count-label">{{ servers.length }}</span>
        </h2>
        <p>每 4 秒更新 · 超过 90 秒未通信标记为离线</p>
      </div>
      <button class="icon-button" aria-label="刷新服务器" @click="refresh">
        <RefreshCw :size="17" />
      </button>
    </div>
    <div class="fleet-toolbar">
      <div class="filter-tabs">
        <button
          v-for="tab in [
            { key: 'all', label: '全部' },
            { key: 'online', label: '在线' },
            { key: 'offline', label: '离线' },
            { key: 'pending', label: '待连接' },
          ]"
          :key="tab.key"
          :class="{ selected: filter === tab.key }"
          @click="filter = tab.key"
        >
          {{ tab.label }}
        </button>
      </div>
      <label class="search-field"
        ><Search :size="16" /><input
          v-model="search"
          :placeholder="publicView ? '搜索服务器名称' : '搜索名称或 IP 地址'"
          aria-label="搜索服务器"
      /></label>
    </div>
    <div v-if="loading" class="empty-state">
      <RefreshCw :size="25" />
      <h3>正在获取服务器…</h3>
    </div>
    <div v-else-if="!visible.length" class="empty-state">
      <Server :size="30" />
      <h3>
        {{
          servers.length
            ? "没有找到匹配的服务器"
            : publicView
              ? "暂无服务器数据"
              : "从连接第一台服务器开始"
        }}
      </h3>
      <p>
        {{
          servers.length
            ? "试试其他名称，或调整状态筛选。"
            : publicView
              ? "管理员添加服务器后，监控数据将显示在这里。"
              : "生成专属连接密钥，启动监控客户端，即可开始监控。"
        }}
      </p>
      <el-button
        v-if="!publicView && !servers.length"
        type="primary"
        @click="addOpen = true"
        >添加服务器</el-button
      >
    </div>
    <div v-else class="server-grid">
      <router-link
        v-for="s in visible"
        :key="s.id"
        :to="(publicView ? '/status/servers/' : '/servers/') + s.id"
        class="server-card"
        ><div class="server-card-top">
          <span class="server-icon"
            ><Server :size="22" :stroke-width="1.5" /></span
          ><span class="status-pill" :class="s.status"
            ><i class="status-dot" :class="s.status"></i
            >{{ statusText[s.status] }}</span
          >
        </div>
        <div class="server-card-name">
          <h3>{{ s.name }}</h3>
          <ArrowUpRight :size="17" />
        </div>
        <div class="server-address">
          <span>{{
            publicView
              ? s.cpu_cores
                ? s.cpu_cores + " 逻辑核心"
                : "等待监控客户端连接"
              : s.ip || "等待监控客户端连接"
          }}</span
          ><span v-if="s.platform"
            >{{ s.platform }} {{ s.platform_version }}</span
          >
        </div>
        <div class="card-metrics">
          <MetricBar label="CPU" :value="s.latest?.cpu" /><MetricBar
            label="内存"
            :value="s.latest?.memory"
          /><MetricBar label="磁盘" :value="s.latest?.disk" />
        </div>
        <div class="card-network">
          <span
            ><ArrowDown :size="13" />{{
              bytes(s.latest?.network_rx_rate, true)
            }}</span
          ><span
            ><ArrowUp :size="13" />{{
              bytes(s.latest?.network_tx_rate, true)
            }}</span
          >
        </div>
        <div class="server-card-foot">
          <span><Clock3 :size="13" />{{ ago(s.last_seen, now) }}</span
          ><span>{{ s.arch || "—" }}</span>
        </div></router-link
      >
    </div>
  </section>
  <section v-if="!publicView" class="events-panel">
    <div class="section-heading">
      <div>
        <h2>最近活动</h2>
        <p>连接事件与资源异常，尽在这里</p>
      </div>
      <router-link to="/logs" class="text-link"
        >查看全部<ArrowUpRight :size="15"
      /></router-link>
    </div>
    <div v-if="!logs.length" class="quiet-state">
      暂无系统事件。监控客户端连接后会在这里显示活动。
    </div>
    <div v-for="item in logs" :key="item.id" class="event-row">
      <span class="log-badge" :class="item.level.toLowerCase()">{{
        item.level
      }}</span
      ><span class="event-message"
        >{{ eventMessage(item.message)
        }}<small>{{
          servers.find((s) => s.id === item.server_id)?.name ||
          `服务器 #${item.server_id}`
        }}</small></span
      ><time>{{ date(item.created_at) }}</time>
    </div>
  </section>
  <div class="data-foot">
    <span
      ><i class="status-dot neutral"></i
      >{{ loaded ? `最近同步 ${ago(loaded, now)}` : "等待同步" }}</span
    ><span>数据保留 7 天 · 监控客户端采集周期 4 秒</span>
  </div>
  <AddServer v-if="!publicView" v-model="addOpen" @created="refresh" />
</template>
