<script setup>
import { ref, computed, onMounted, onUnmounted, watch } from "vue";
import { useRoute } from "vue-router";
import { ShieldCheck, RefreshCw, Download, Bell } from "lucide-vue-next";
import { ElMessage } from "element-plus";
import { api, request } from "../api";
import { user } from "../state";
import SecuritySetup from "../components/SecuritySetup.vue";

const route = useRoute();
const servers = ref([]),
  selected = ref(Number(route.query.server_id) || null);
const days = ref(7),
  severity = ref(""),
  page = ref(1),
  tab = ref("findings");
const overview = ref(null),
  findings = ref([]),
  events = ref([]),
  alerts = ref([]),
  reads = ref([]),
  scans = ref([]),
  scores = ref([]),
  audit = ref([]);
const serverRisks = ref([]);
const setupOpen = ref(false);
const selectedServer = computed(() =>
  servers.value.find((s) => s.id === selected.value),
);
const loading = ref(false),
  starting = ref(false),
  error = ref(""),
  detail = ref(null),
  report = ref(null),
  policy = ref(null),
  options = ref(null),
  muteHours = ref(0);
const levels = {
  critical: "严重",
  high: "高风险",
  medium: "中风险",
  low: "低风险",
  info: "提示",
  unknown: "未知",
};
const states = {
  pass: "已检测",
  risk: "风险",
  unknown: "待确认",
  error: "检测失败",
  not_installed: "未安装",
  not_authorized: "未授权",
};
const workflow = {
  pending: "待处理",
  confirmed: "已确认",
  resolved: "已处理",
  false_positive: "误报",
};
const scanStates = {
  queued: "排队中",
  running: "检测中",
  success: "已完成",
  failed: "失败",
  expired: "已过期",
  cancelled: "已取消",
};
const dimensions = {
  system: "系统与账户",
  network: "SSH 与网络",
  website: "网站与 TLS",
  docker: "Docker",
  logs: "日志与事件",
};
let controller,
  generation = 0,
  timer,
  ticks = 0;
const active = computed(() =>
  ["queued", "running"].includes(overview.value?.latest_scan?.status),
);
const grade = computed(() => {
  try {
    return JSON.parse(overview.value?.latest_score?.grade_json || "{}");
  } catch {
    return {};
  }
});
const ordered = computed(() => [...scores.value].reverse());
const scoreDots = computed(() =>
  ordered.value
    .map((s, i) => ({
      ...s,
      x: 20 + (i * 560) / Math.max(1, ordered.value.length - 1),
      y: 130 - (s.score || 0) * 1.1,
    }))
    .filter((s) => s.score != null),
);
const scorePoints = computed(() => {
  const segments = [[]];
  ordered.value.forEach((s, i) => {
    if (s.score == null) segments.push([]);
    else
      segments
        .at(-1)
        .push(
          `${20 + (i * 560) / Math.max(1, ordered.value.length - 1)},${130 - s.score * 1.1}`,
        );
  });
  return segments.filter((v) => v.length).map((v) => v.join(" "));
});
const trend = computed(() => {
  const groups = new Map();
  for (const e of events.value) {
    const d = new Date(e.created_at).toLocaleDateString();
    groups.set(d, (groups.get(d) || 0) + 1);
  }
  return [...groups].reverse();
});
const unread = (id) => !reads.value.some((r) => r.alert_id === id);
const date = (v) =>
  v && !String(v).startsWith("0001-") ? new Date(v).toLocaleString() : "—";
const endpoint = (p, more = {}) =>
  "/security/" +
  p +
  "?" +
  new URLSearchParams({
    server_id: selected.value,
    days: days.value,
    page: page.value,
    ...(severity.value ? { severity: severity.value } : {}),
    ...more,
  });
async function load() {
  if (!selected.value) return;
  const current = ++generation;
  controller?.abort();
  controller = new AbortController();
  const signal = controller.signal;
  loading.value = true;
  error.value = "";
  try {
    const r = await Promise.all(
      [
        "overview",
        "findings",
        "events",
        "alerts",
        "scans",
        "scores",
        "servers",
      ].map((p) => request(endpoint(p), { signal })),
    );
    if (current !== generation) return;
    [overview.value] = r;
    findings.value = r[1].items;
    events.value = r[2].items;
    alerts.value = r[3].items;
    reads.value = r[3].read;
    scans.value = r[4].items;
    scores.value = r[5].items;
    serverRisks.value = r[6].items;
    const log = await request("/operation-logs?server_id=" + selected.value, {
      signal,
    });
    if (current === generation)
      audit.value = log.items.filter(
        (v) =>
          v.action.startsWith("security.") || v.action.includes("/security/"),
      );
  } catch (e) {
    if (e.name !== "AbortError" && current === generation)
      error.value = e.message;
  } finally {
    if (current === generation) loading.value = false;
  }
}
async function loadPolicy() {
  if (!selected.value) return;
  const id = selected.value;
  try {
    const r = await request(endpoint("rules"));
    if (selected.value === id) {
      policy.value = r.policy;
      options.value = r.options;
      muteHours.value = 0;
    }
  } catch (e) {
    ElMessage.error(e.message);
  }
}
async function startScan() {
  starting.value = true;
  try {
    await request("/security/scans", {
      method: "POST",
      body: JSON.stringify({ server_id: selected.value }),
    });
    ElMessage.success("已排队，客户端将通过原有连接领取只读检测任务");
    await load();
  } catch (e) {
    ElMessage.error(e.message);
  } finally {
    starting.value = false;
  }
}
async function review(status) {
  const finding = detail.value;
  if (!finding) return;
  try {
    await request("/security/findings/" + finding.id, {
      method: "PATCH",
      body: JSON.stringify({ server_id: finding.server_id, status }),
    });
    detail.value = { ...finding, workflow: status };
    await load();
  } catch (e) {
    ElMessage.error(e.message);
  }
}
async function readAlert(alert) {
  try {
    await request("/security/alerts/" + alert.id, {
      method: "PATCH",
      body: JSON.stringify({ server_id: alert.server_id }),
    });
    await load();
  } catch (e) {
    ElMessage.error(e.message);
  }
}
async function viewReport(scan) {
  try {
    const r = await request(
      "/security/reports/" + scan.id + "?server_id=" + scan.server_id,
    );
    report.value = r;
  } catch (e) {
    ElMessage.error(e.message);
  }
}
function downloadReport() {
  if (!report.value) return;
  const url = URL.createObjectURL(
    new Blob([JSON.stringify(report.value, null, 2)], {
      type: "application/json",
    }),
  );
  const link = document.createElement("a");
  link.href = url;
  link.download = `security-report-${report.value.scan.id}.json`;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
async function savePolicy() {
  try {
    await request("/security/rules", {
      method: "PUT",
      body: JSON.stringify({
        server_id: selected.value,
        options: options.value,
        retention_days: policy.value.retention_days,
        mute_hours: muteHours.value,
        interval_hours: policy.value.interval_hours,
      }),
    });
    ElMessage.success("已保存；定时扫描受当前会话有效期约束");
    await loadPolicy();
  } catch (e) {
    ElMessage.error(e.message);
  }
}
watch(selected, () => {
  page.value = 1;
  detail.value = null;
  report.value = null;
  policy.value = null;
  options.value = null;
  overview.value = null;
  findings.value = [];
  events.value = [];
  alerts.value = [];
  scans.value = [];
  scores.value = [];
  load();
  loadPolicy();
});
watch([days, severity, page], load);
onMounted(async () => {
  try {
    servers.value = await api.servers();
    if (!servers.value.some((s) => s.id === selected.value))
      selected.value = servers.value[0]?.id || null;
    else {
      await load();
      await loadPolicy();
    }
  } catch (e) {
    error.value = e.message;
  }
  timer = setInterval(() => {
    ticks++;
    if (active.value || ticks % 6 === 0) load();
  }, 5000);
});
onUnmounted(() => {
  generation++;
  controller?.abort();
  clearInterval(timer);
});
</script>

<template>
  <div class="security-center">
    <div class="page-heading">
      <div>
        <span class="eyebrow">CLOUDEASE / V0.6</span>
        <h1><ShieldCheck :size="30" /> 安全中心</h1>
        <p>只读检测、风险解释与人工确认。每一项结论都可查看依据。</p>
      </div>
      <div class="security-tools">
        <el-select v-model="selected" placeholder="选择服务器"
          ><el-option
            v-for="s in servers"
            :key="s.id"
            :value="s.id"
            :label="
              s.name + ' · ' + (s.status === 'online' ? '在线' : '离线')
            " /></el-select
        ><el-button :disabled="loading" @click="load"
          ><RefreshCw :size="15" />刷新</el-button
        ><el-button :disabled="!selectedServer" @click="setupOpen = true">{{
          overview?.enabled ? "检测配置" : "开启安全检测"
        }}</el-button
        ><el-button
          type="primary"
          :disabled="
            !overview?.enabled || !overview?.online || active || starting
          "
          @click="startScan"
          >{{ active ? "正在检测" : "开始只读检测" }}</el-button
        >
      </div>
    </div>
    <el-alert
      v-if="error"
      :title="error"
      type="error"
      :closable="false"
      class="page-error"
    />
    <el-alert
      v-if="servers.find((s) => s.id === selected)?.name?.includes('模拟')"
      title="本地测试夹具：此服务器的 Linux、SSH 与证书输入为明确模拟，供验证页面和任务流程，不能视为真实主机检测。"
      type="warning"
      :closable="false"
      class="page-error"
    />
    <el-alert
      v-if="!overview?.enabled && selected"
      title="此客户端尚未明确启用安全检测。需要 v0.4 Linux 客户端，并选择运维管理与只读安全检测；普通监控、Windows 及容器兼容模式保持原权限。"
      type="info"
      :closable="false"
      class="page-error"
    />
    <div v-if="!servers.length" class="panel muted">
      请先添加监控服务器。安全中心不会自动连接或扫描外部资产。
    </div>
    <template v-else>
      <div class="security-stats">
        <section class="panel">
          <span class="small muted">规则参考评分</span>
          <div class="security-score">
            {{ overview?.latest_score?.score ?? "—" }}<small>/ 100</small>
          </div>
          <p>
            {{ levels[grade.risk] || "尚未检测" }}
            <span v-if="overview?.latest_score?.delta != null"
              >· 变化 {{ overview.latest_score.delta > 0 ? "+" : ""
              }}{{ overview.latest_score.delta }}</span
            >
          </p>
          <p class="small muted">分数不是安全保证；未检测项不算通过。</p>
        </section>
        <section class="panel">
          <span class="small muted">风险分布</span>
          <div class="risk-counts">
            <span
              v-for="key in ['critical', 'high', 'medium', 'low']"
              :key="key"
              ><b>{{ grade[key] || 0 }}</b
              >{{ levels[key] }}</span
            >
          </div>
          <p class="small muted">仅按检测依据计算，人工标记不改变评分。</p>
        </section>
        <section class="panel">
          <span class="small muted">检测覆盖</span>
          <div class="security-score">
            {{ grade.checked || 0 }}<small>项已检测</small>
          </div>
          <p>
            未确认 {{ grade.unknown || 0 }} · 未安装
            {{ grade.not_installed || 0 }}
          </p>
          <p class="small muted">权限不足、失败和未授权独立记录。</p>
        </section>
        <section class="panel">
          <span class="small muted">最近检测</span>
          <h3>{{ scanStates[overview?.latest_scan?.status] || "未开始" }}</h3>
          <p class="small">{{ date(overview?.latest_scan?.updated_at) }}</p>
          <p v-if="overview?.latest_scan?.error" class="small">
            {{ overview.latest_scan.error }}
          </p>
          <button class="text-link" @click="tab = 'alerts'">
            <Bell :size="14" /> 未读通知 {{ overview?.unread_alerts || 0 }}
          </button>
        </section>
      </div>
      <div class="security-filters">
        <el-select v-model="days" aria-label="时间范围"
          ><el-option
            v-for="n in [1, 7, 30, 90]"
            :key="n"
            :value="n"
            :label="'最近 ' + n + ' 天'" /></el-select
        ><el-select
          v-model="severity"
          clearable
          placeholder="全部风险等级"
          aria-label="风险等级"
          ><el-option
            v-for="(label, key) in levels"
            v-show="key !== 'unknown'"
            :key="key"
            :value="key"
            :label="label" /></el-select
        ><span class="small muted"
          >单页最多 50 条；明细默认展示最近一次完整扫描。</span
        >
      </div>
      <div class="security-trends">
        <section class="panel">
          <h3>评分变化</h3>
          <svg
            v-if="ordered.some((s) => s.score != null)"
            viewBox="0 0 600 160"
            role="img"
            aria-label="安全评分历史趋势"
          >
            <path d="M20 20V130H580" fill="none" stroke="var(--border)" />
            <polyline
              v-for="(points, i) in scorePoints"
              :key="i"
              :points="points"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
            />
            <circle
              v-for="s in scoreDots"
              :key="s.id"
              :cx="s.x"
              :cy="s.y"
              r="3"
              fill="currentColor"
            >
              <title>{{ date(s.created_at) }}：{{ s.score }} 分</title>
            </circle>
          </svg>
          <p v-else class="muted">尚无可计分的检测结果。</p>
          <p class="small muted">
            保留未确认项；仅当前时间范围最近 50 次检测。
          </p>
        </section>
        <section class="panel">
          <h3>异常事件趋势</h3>
          <div v-if="trend.length" class="event-bars">
            <div v-for="[day, count] in trend" :key="day">
              <span>{{ day }}</span
              ><i
                :style="{
                  width:
                    Math.max(
                      4,
                      (count / Math.max(...trend.map((v) => v[1]))) * 60,
                    ) + '%',
                }"
              /><b>{{ count }}</b>
            </div>
          </div>
          <p v-else class="muted">当前筛选范围没有异常事件。</p>
          <p class="small muted">
            按本页最多 50 条事件统计，异常不等于确认攻击。
          </p>
        </section>
      </div>
      <section class="panel">
        <h3>服务器风险概况</h3>
        <el-table
          :data="serverRisks.filter((s) => !severity || s.risk === severity)"
          empty-text="当前筛选没有服务器"
          @row-click="selected = $event.server_id"
          ><el-table-column
            prop="name"
            label="服务器"
            min-width="160"
          /><el-table-column label="连接状态" width="110"
            ><template #default="{ row }">{{
              row.online ? "在线" : "离线"
            }}</template></el-table-column
          ><el-table-column label="规则参考评分" width="130"
            ><template #default="{ row }">{{
              row.score ?? "—"
            }}</template></el-table-column
          ><el-table-column label="风险" width="110"
            ><template #default="{ row }">{{
              levels[row.risk] || "未检测"
            }}</template></el-table-column
          ><el-table-column label="最近完成" min-width="180"
            ><template #default="{ row }">{{
              date(row.checked_at)
            }}</template></el-table-column
          ></el-table
        >
        <p class="small muted">
          最近一次结果，最多 200
          台；点击切换服务器。未检测显示未知，离线状态不代表风险消失。
        </p>
      </section>
      <section class="panel">
        <el-tabs v-model="tab">
          <el-tab-pane label="检测明细与建议" name="findings"
            ><el-table
              :data="findings"
              empty-text="暂无检测明细"
              @row-click="detail = $event"
              ><el-table-column
                prop="title"
                label="检测项"
                min-width="200"
              /><el-table-column label="状态" width="120"
                ><template #default="{ row }">{{
                  states[row.status]
                }}</template></el-table-column
              ><el-table-column label="等级" width="100"
                ><template #default="{ row }"
                  ><span :class="['risk-label', row.severity]">{{
                    levels[row.severity]
                  }}</span></template
                ></el-table-column
              ><el-table-column label="处理" width="110"
                ><template #default="{ row }">{{
                  workflow[row.workflow]
                }}</template></el-table-column
              ><el-table-column
                prop="evidence"
                label="依据"
                min-width="260"
                show-overflow-tooltip
              /><el-table-column label="检测时间" width="180"
                ><template #default="{ row }">{{
                  date(row.created_at)
                }}</template></el-table-column
              ></el-table
            >
            <p class="small muted">
              点击一项查看影响、原因和建议；误报标记不会删除原始记录或提高评分。
            </p></el-tab-pane
          >
          <el-tab-pane label="异常日志事件" name="events"
            ><el-table :data="events" empty-text="暂无异常日志事件"
              ><el-table-column label="等级" width="100"
                ><template #default="{ row }">{{
                  levels[row.severity]
                }}</template></el-table-column
              ><el-table-column
                prop="message"
                label="异常摘要"
                min-width="300"
              /><el-table-column label="观测时间" width="180"
                ><template #default="{ row }">{{
                  date(row.observed_at)
                }}</template></el-table-column
              ></el-table
            >
            <p class="small muted">
              只存聚合摘要，不存原始日志和来源 IP；NAT
              或代理来源可能包含多个正常用户。
            </p></el-tab-pane
          >
          <el-tab-pane label="站内告警" name="alerts"
            ><el-table :data="alerts" empty-text="暂无站内告警"
              ><el-table-column label="状态" width="85"
                ><template #default="{ row }">{{
                  unread(row.id) ? "未读" : "已读"
                }}</template></el-table-column
              ><el-table-column label="等级" width="100"
                ><template #default="{ row }">{{
                  levels[row.severity]
                }}</template></el-table-column
              ><el-table-column
                prop="message"
                label="通知"
                min-width="300"
              /><el-table-column label="时间" width="180"
                ><template #default="{ row }">{{
                  date(row.created_at)
                }}</template></el-table-column
              ><el-table-column width="110"
                ><template #default="{ row }"
                  ><el-button
                    v-if="unread(row.id)"
                    size="small"
                    @click="readAlert(row)"
                    >标为已读</el-button
                  ></template
                ></el-table-column
              ></el-table
            ></el-tab-pane
          >
          <el-tab-pane label="扫描与报告" name="reports"
            ><el-table :data="scans" empty-text="暂无扫描任务"
              ><el-table-column
                prop="id"
                label="编号"
                width="80"
              /><el-table-column label="状态" width="120"
                ><template #default="{ row }">{{
                  scanStates[row.status]
                }}</template></el-table-column
              ><el-table-column label="时间" min-width="180"
                ><template #default="{ row }">{{
                  date(row.created_at)
                }}</template></el-table-column
              ><el-table-column
                prop="error"
                label="失败原因"
                min-width="200"
              /><el-table-column width="110"
                ><template #default="{ row }"
                  ><el-button size="small" @click="viewReport(row)"
                    >查看报告</el-button
                  ></template
                ></el-table-column
              ></el-table
            ></el-tab-pane
          >
          <el-tab-pane label="规则与计划" name="rules"
            ><template v-if="options && policy"
              ><p class="muted">
                规则只读检测，不执行修复。权重仅对已检测维度归一化。Webhook
                未启用。
              </p>
              <el-form label-position="top" class="security-policy"
                ><el-form-item
                  v-for="(label, key) in dimensions"
                  :key="key"
                  :label="label + '权重'"
                  ><el-input-number
                    v-model="options.weights[key]"
                    :min="1"
                    :max="100"
                    :disabled="user?.role !== 'admin'" /></el-form-item
                ><el-form-item label="日志窗口（秒）"
                  ><el-input-number
                    v-model="options.window_seconds"
                    :min="60"
                    :max="3600"
                    :disabled="user?.role !== 'admin'" /></el-form-item
                ><el-form-item label="SSH 失败阈值"
                  ><el-input-number
                    v-model="options.ssh_failures"
                    :min="3"
                    :max="10000"
                    :disabled="user?.role !== 'admin'" /></el-form-item
                ><el-form-item label="同来源请求阈值"
                  ><el-input-number
                    v-model="options.requests"
                    :min="10"
                    :max="100000"
                    :disabled="user?.role !== 'admin'" /></el-form-item
                ><el-form-item label="错误响应比例（%）"
                  ><el-input-number
                    v-model="options.error_percent"
                    :min="10"
                    :max="100"
                    :disabled="user?.role !== 'admin'" /></el-form-item
                ><el-form-item label="敏感路径请求阈值"
                  ><el-input-number
                    v-model="options.sensitive_requests"
                    :min="3"
                    :max="10000"
                    :disabled="user?.role !== 'admin'" /></el-form-item
                ><el-form-item label="安全数据保留（天）"
                  ><el-input-number
                    v-model="policy.retention_days"
                    :min="1"
                    :max="365"
                    :disabled="user?.role !== 'admin'" /></el-form-item
                ><el-form-item label="静默新通知（小时，0 取消）"
                  ><el-input-number
                    v-model="muteHours"
                    :min="0"
                    :max="168"
                    :disabled="user?.role !== 'admin'" /></el-form-item
                ><el-form-item label="定时扫描（0 关闭，6–168 小时）"
                  ><el-input-number
                    v-model="policy.interval_hours"
                    :min="0"
                    :max="168"
                    :disabled="user?.role !== 'admin'" /></el-form-item
              ></el-form>
              <p class="small muted">
                当前静默至
                {{
                  date(policy.mute_until)
                }}。计划依赖保存者会话，退出、会话过期或降权后停止。监控指标保留
                30 天不受影响。
              </p>
              <el-button
                v-if="user?.role === 'admin'"
                type="primary"
                @click="savePolicy"
                >保存规则</el-button
              ></template
            ></el-tab-pane
          >
          <el-tab-pane label="安全审计" name="audit"
            ><el-table :data="audit" empty-text="暂无近期安全审计"
              ><el-table-column
                prop="user"
                label="用户"
                width="100"
              /><el-table-column
                prop="action"
                label="动作"
                min-width="200"
              /><el-table-column
                prop="status"
                label="结果"
                width="100"
              /><el-table-column label="时间" min-width="180"
                ><template #default="{ row }">{{
                  date(row.created_at)
                }}</template></el-table-column
              ></el-table
            >
            <p class="small muted">
              最近一页运维审计中的安全操作；完整记录见日志页面。
            </p></el-tab-pane
          >
        </el-tabs>
        <div class="security-pagination">
          <el-button :disabled="page <= 1" @click="page--">上一页</el-button
          ><span class="small muted">第 {{ page }} 页</span
          ><el-button
            :disabled="
              { findings, events, alerts, scans, scores }[
                tab === 'reports' ? 'scans' : tab
              ]?.length < 50 || ['rules', 'audit'].includes(tab)
            "
            @click="page++"
            >下一页</el-button
          >
        </div>
      </section>
    </template>
    <el-dialog
      v-model="setupOpen"
      title="开启只读安全检测"
      width="min(760px, 94vw)"
      destroy-on-close
    >
      <SecuritySetup
        v-if="selectedServer"
        :key="selectedServer.id"
        :server="selectedServer"
        :enabled="!!overview?.enabled"
      />
      <template #footer>
        <el-button @click="setupOpen = false">关闭</el-button>
        <el-button
          type="primary"
          @click="
            setupOpen = false;
            load();
          "
          >刷新启用状态</el-button
        >
      </template>
    </el-dialog>
    <el-drawer
      :model-value="!!detail"
      title="检测依据与处理建议"
      size="min(620px, 94vw)"
      @close="detail = null"
      ><template v-if="detail"
        ><h2>{{ detail.title }}</h2>
        <p>
          {{ servers.find((s) => s.id === detail.server_id)?.name }} ·
          {{ levels[detail.severity] }} · {{ states[detail.status] }}
        </p>
        <p class="small muted">
          {{ date(detail.created_at) }} · {{ detail.rule }}
        </p>
        <h3>发现依据</h3>
        <p class="security-evidence">{{ detail.evidence }}</p>
        <h3>风险原因</h3>
        <p>{{ detail.reason }}</p>
        <h3>修复建议</h3>
        <p>{{ detail.advice }}</p>
        <h3>人工处理状态</h3>
        <el-select :model-value="detail.workflow" @change="review"
          ><el-option
            v-for="(label, key) in workflow"
            :key="key"
            :value="key"
            :label="label"
        /></el-select>
        <p class="small muted">
          记录到运维审计；原始检测结论和计分依据保持不变。
        </p></template
      ></el-drawer
    >
    <el-drawer
      :model-value="!!report"
      title="安全检测报告"
      size="min(760px, 94vw)"
      @close="report = null"
      ><template v-if="report"
        ><el-button @click="downloadReport"
          ><Download :size="15" />导出 JSON 报告</el-button
        >
        <p>
          {{ scanStates[report.scan.status] }} ·
          {{ date(report.scan.created_at) }}
        </p>
        <p v-if="!report.report" class="muted">扫描尚未完成，没有检测结果。</p>
        <template v-else
          ><p class="muted">
            {{
              report.report.scope
            }}。本报告是规则参考，不能证明不存在其他风险。
          </p>
          <article
            v-for="item in report.report.checks"
            :key="item.rule"
            class="report-check"
          >
            <h3>{{ item.title }} · {{ states[item.status] }}</h3>
            <p>{{ item.evidence }}</p>
            <p class="muted">{{ item.advice }}</p>
          </article></template
        ></template
      ></el-drawer
    >
  </div>
</template>

<style scoped>
.security-center h1 {
  display: flex;
  align-items: center;
  gap: 8px;
}
.security-tools,
.security-filters {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.security-tools .el-select {
  width: 230px;
}
.security-filters {
  margin: 24px 0;
}
.security-filters .el-select {
  width: 160px;
}
.security-stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 18px;
}
.security-stats .panel {
  margin: 0;
  padding: 24px;
}
.security-score {
  font-size: 40px;
  line-height: 1.7;
  font-weight: 600;
}
.security-score small {
  font-size: 13px;
  font-weight: 400;
  margin-left: 8px;
  color: var(--muted);
}
.risk-counts {
  display: flex;
  gap: 20px;
  margin: 24px 0;
}
.risk-counts span {
  display: grid;
  gap: 6px;
  font-size: 12px;
  color: var(--muted);
}
.risk-counts b {
  font-size: 25px;
  color: var(--text);
}
.security-trends {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 20px;
}
.security-trends svg {
  width: 100%;
  height: 160px;
  color: var(--text);
}
.event-bars {
  min-height: 160px;
  display: grid;
  align-content: center;
  gap: 8px;
}
.event-bars div {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 12px;
}
.event-bars i {
  display: inline-block;
  height: 9px;
  background: var(--surface-alt);
  border-radius: 6px;
}
.risk-label {
  font-size: 12px;
  border-radius: 6px;
  padding: 4px 8px;
  background: var(--surface-alt);
}
.risk-label.high,
.risk-label.critical {
  color: var(--danger);
  background: var(--surface-alt);
}
.risk-label.medium {
  color: var(--danger);
  background: var(--surface-alt);
}
.security-evidence {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  padding: 18px;
  background: var(--surface-alt);
  border-radius: 12px;
}
.security-policy {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0 20px;
}
.security-pagination {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 12px;
  margin-top: 20px;
}
.report-check {
  padding: 12px 0;
  border-bottom: 1px solid var(--border);
}
.security-stats button {
  display: flex;
  align-items: center;
  gap: 6px;
}
@media (max-width: 1250px) {
  .security-stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 700px) {
  .security-stats,
  .security-trends,
  .security-policy {
    grid-template-columns: 1fr;
  }
  .security-tools .el-select {
    width: 100%;
  }
  .security-filters {
    gap: 8px;
  }
  .security-filters .el-select {
    width: calc(50% - 4px);
  }
}
</style>
