<script setup>
import {
  computed,
  ref,
  onMounted,
  onUnmounted,
  watch,
  defineAsyncComponent,
} from "vue";
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
  Map,
} from "lucide-vue-next";
import { api, publicApi } from "../api";
import { createRefresh } from "../refresh";
const props = defineProps({ publicView: Boolean, mapExpanded: Boolean });
import { canOperate } from "../management";
import { revision } from "../state";
import { ago, percent, bytes, statusText, date, eventMessage } from "../format";
import ServerNodeCard from "../components/ServerNodeCard.vue";
import MonitorSummary from "../components/MonitorSummary.vue";
import { dashboard } from "../appearance";
import { countryName } from "../geography";
import { X, ChevronDown, ChevronUp } from "lucide-vue-next";
import AddServer from "../components/AddServer.vue";
const ServerMap = defineAsyncComponent(
  () => import("../components/ServerMap.vue"),
);
const route = useRoute(),
  servers = ref([]),
  logs = ref([]),
  loading = ref(true),
  error = ref(""),
  addOpen = ref(false),
  mapOpen = ref(props.mapExpanded),
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
      `${s.name} ${props.publicView ? "" : s.ip || ""} ${s.platform || ""} ${s.os || ""} ${s.country_code || ""} ${countryName(s.country_code)}`
        .toLowerCase()
        .includes(search.value.toLowerCase()),
  ),
);
const serverListView = computed(
  () => !props.publicView && route.path === "/servers",
);
const title = computed(() =>
  serverListView.value ? "服务器" : "基础设施概览",
);
const adminMapOpen = ref(dashboard.mapExpanded);
watch(
  () => dashboard.mapExpanded,
  (value) => {
    adminMapOpen.value = value;
  },
);
watch(
  () => props.mapExpanded,
  (value) => {
    mapOpen.value = value;
  },
);
let poll, clock;
const refresh = createRefresh(
  (signal) =>
    Promise.all([
      (props.publicView ? publicApi : api).servers({ signal }),
      props.publicView || serverListView.value
        ? Promise.resolve({ items: [] })
        : api.logs({ limit: 5 }, { signal }),
    ]),
  ([items, events]) => {
    servers.value = items;
    logs.value = events.items;
    error.value = "";
    loaded.value = Date.now();
  },
  (e) => {
    error.value = e.message;
  },
  () => {
    loading.value = false;
  },
);
onMounted(() => {
  refresh();
  poll = setInterval(refresh, 4000);
  clock = setInterval(() => (now.value = Date.now()), 1000);
});
onUnmounted(() => {
  refresh.stop();
  clearInterval(poll);
  clearInterval(clock);
});
watch(revision, refresh);
watch(serverListView, () => refresh(true));
</script>
<template>
  <div v-if="!publicView" class="page-heading">
    <div>
      <span class="eyebrow">{{
        serverListView ? "SERVER MANAGEMENT" : "INFRASTRUCTURE OVERVIEW"
      }}</span>
      <h1>{{ title }}</h1>
      <p>
        {{
          serverListView
            ? "管理监控服务器，查看运行状态和资源使用情况。"
            : "关注关键指标，让每一台服务器保持最佳状态。"
        }}
      </p>
    </div>
    <el-button
      v-if="canOperate"
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
  <MonitorSummary
    v-if="publicView"
    :servers="servers"
    :now="now"
    :loading="loading"
  />
  <div v-if="!publicView && !serverListView" class="stats-grid">
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

  <div v-if="publicView && mapOpen" id="server-distribution-map">
    <ServerMap :servers="servers" public-view />
  </div>
  <template v-if="!publicView && !serverListView">
    <section
      v-for="section in dashboard.sections"
      :key="section"
      class="dashboard-module"
      :data-module="section"
    >
      <template v-if="section === 'map'"
        ><button
          class="module-toggle"
          :aria-expanded="adminMapOpen"
          aria-controls="admin-server-map"
          @click="adminMapOpen = !adminMapOpen"
        >
          <Map :size="17" />服务器地图<component
            :is="adminMapOpen ? ChevronUp : ChevronDown"
            :size="17"
          />
        </button>
        <div v-if="adminMapOpen" id="admin-server-map">
          <ServerMap
            :servers="servers"
            :editable="canOperate"
            @updated="refresh"
          /></div
      ></template>
      <section v-else class="events-panel">
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
    </section>
  </template>
  <section v-if="publicView || serverListView" class="fleet-section node-wall">
    <div class="node-tools">
      <label class="search-field"
        ><Search :size="17" /><input
          v-model="search"
          :placeholder="
            publicView
              ? '搜索节点名称、地区、系统…'
              : '搜索节点名称、地区、系统或 IP…'
          "
          aria-label="搜索服务器" /><button
          v-if="search"
          class="clear-search"
          aria-label="清除搜索"
          @click="search = ''"
        >
          <X :size="15" /></button
      ></label>
      <div class="choice-row">
        <button class="icon-button" aria-label="刷新服务器" @click="refresh">
          <RefreshCw :size="16" /></button
        >
      </div>
    </div>
    <div class="node-filter-row">
      <p class="node-count">
        共 {{ servers.length }} 个服务器，{{ online.length }} 个在线<small
          v-if="search || filter !== 'all'"
          >匹配 {{ visible.length }} 台</small
        >
      </p>
      <div class="filter-tabs" aria-label="服务器状态筛选">
        <button
          v-for="tab in [
            { key: 'all', label: '全部' },
            { key: 'online', label: '在线' },
            { key: 'offline', label: '离线' },
            { key: 'pending', label: '待连接' },
          ]"
          :key="tab.key"
          :class="{ selected: filter === tab.key }"
          :aria-pressed="filter === tab.key"
          @click="filter = tab.key"
        >
          {{ tab.label }}
        </button>
      </div>
    </div>
    <div v-if="loading" class="empty-state">
      <RefreshCw :size="25" />
      <h3>正在获取服务器…</h3>
    </div>
    <div v-else-if="!visible.length" class="empty-state">
      <Server :size="30" />
      <h3>{{ servers.length ? "没有找到匹配的服务器" : "暂无服务器数据" }}</h3>
      <p>
        {{
          servers.length
            ? "试试其他名称、地区、系统，或调整状态筛选。"
            : "监控客户端连接后，数据将显示在这里。"
        }}
      </p>
      <el-button
        v-if="!publicView && canOperate && !servers.length"
        type="primary"
        @click="addOpen = true"
        >添加服务器</el-button
      >
    </div>
    <div v-else class="node-grid compact">
      <ServerNodeCard
        v-for="s in visible"
        :key="s.id"
        :server="s"
        :public-view="publicView"
        :now="now"
      />
    </div>
  </section>
  <div class="data-foot">
    <span
      ><i class="status-dot neutral"></i
      >{{ loaded ? `最近同步 ${ago(loaded, now)}` : "等待同步" }}</span
    ><span>数据保留 30 天 · 监控客户端采集周期 4 秒</span>
  </div>
  <AddServer
    v-if="!publicView && canOperate"
    v-model="addOpen"
    @created="refresh"
  />
</template>
