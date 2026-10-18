<script setup>
import { ref, watch, computed } from "vue";
import {
  ShieldCheck,
  Clock,
  AlertTriangle,
  ExternalLink,
} from "lucide-vue-next";
import { ElMessage } from "element-plus";
import ManagementHeader from "../components/ManagementHeader.vue";
import { useManagement } from "../management";
import { request } from "../api";
const scope = useManagement(),
  certificates = ref([]),
  sites = ref([]),
  detail = ref(null),
  open = ref(false);
const ready = computed(
  () =>
    scope.info?.server.status === "online" &&
    scope.info?.capabilities.websites?.enabled,
);
const alerts = computed(() =>
  certificates.value.filter((c) => c.warning || c.status === "异常"),
);
async function load() {
  certificates.value = [];
  sites.value = [];
  if (!scope.info) return;
  const id = scope.selected;
  try {
    const [ssl, w] = await Promise.all([
      request(`/ssl?server_id=${id}`),
      request(`/websites?server_id=${id}`),
    ]);
    if (id !== scope.selected) return;
    certificates.value = ssl.items;
    sites.value = w.items;
  } catch (e) {
    if (id === scope.selected) scope.error = e.message;
  }
}
watch(() => scope.info, load);
async function check(site) {
  try {
    await scope.execute("/ssl/check", { website_id: site.id }, "POST");
    ElMessage.success("SSL 与域名检测完成");
    await load();
  } catch {}
}
const date = (v) => (v ? new Date(v).toLocaleString() : "—");
</script>
<template>
  <el-alert
    v-if="scope.info?.capabilities.websites?.demo"
    title="本地演示证书：用于测试页面、权限与检测流程，证书及 HTTPS 检测数据为模拟结果。"
    type="warning"
    :closable="false"
    show-icon
    class="page-error"
  />
  <ManagementHeader
    title="SSL 与 HTTPS"
    description="验证证书链与域名，提前了解过期风险和 HTTPS 可用性。"
    :scope="scope"
  />
  <div class="website-summary ssl-summary">
    <div class="website-summary-card">
      <ShieldCheck :size="20" /><small>已检测证书</small
      ><strong
        >{{ certificates.length
        }}<span>只读检测，保留完整证书校验</span></strong
      >
    </div>
    <div class="website-summary-card">
      <Clock :size="20" /><small>30 天内到期</small
      ><strong
        >{{
          certificates.filter((c) => c.status === "有效" && c.days_left <= 30)
            .length
        }}<span>提前安排续期</span></strong
      >
    </div>
    <div class="website-summary-card">
      <AlertTriangle :size="20" /><small>高危或异常</small
      ><strong
        >{{
          certificates.filter(
            (c) =>
              c.status === "异常" || (c.status === "有效" && c.days_left <= 7),
          ).length
        }}<span>7 天内到期或 HTTPS 验证失败</span></strong
      >
    </div>
  </div>
  <el-alert
    v-if="!ready && scope.info"
    title="需要 v0.3 Linux 客户端的网站白名单。请先在网站管理中登记站点。"
    type="info"
    :closable="false"
    class="page-error"
  />
  <p v-for="c in alerts" :key="c.id" class="website-warning">
    {{ c.domain }} · {{ c.warning || c.error || "HTTPS 验证异常" }}
  </p>
  <div class="panel">
    <div class="section-heading">
      <h2>证书与站点</h2>
      <router-link
        class="text-link"
        :to="`/websites?server_id=${scope.selected || ''}`"
        >前往网站管理 →</router-link
      >
    </div>
    <el-table
      :data="sites"
      empty-text="尚未登记网站。请先在网站管理中添加授权站点。"
      ><el-table-column
        prop="domain"
        label="域名"
        min-width="200"
      /><el-table-column label="状态" width="110"
        ><template #default="{ row }">{{
          certificates.find((c) => c.website_id === row.id)?.status || "未检测"
        }}</template></el-table-column
      ><el-table-column label="证书机构" min-width="200"
        ><template #default="{ row }">{{
          certificates.find((c) => c.website_id === row.id)?.issuer || "—"
        }}</template></el-table-column
      ><el-table-column label="剩余天数" width="120"
        ><template #default="{ row }">{{
          certificates.find((c) => c.website_id === row.id)?.not_after
            ? certificates.find((c) => c.website_id === row.id).days_left
            : "—"
        }}</template></el-table-column
      ><el-table-column label="操作" width="180"
        ><template #default="{ row }"
          ><el-button
            size="small"
            :disabled="!certificates.find((c) => c.website_id === row.id)"
            @click="
              detail = certificates.find((c) => c.website_id === row.id);
              open = true;
            "
            >查看</el-button
          ><el-button
            size="small"
            :disabled="!ready || scope.busy || !scope.canOperate"
            @click="check(row)"
            >检测</el-button
          ></template
        ></el-table-column
      ></el-table
    >
  </div>
  <p class="small muted website-note">
    客户端约每 30 秒刷新站点检测；30 天内到期提醒，7
    天内高危。这里只检测现有证书，不申请、签发或自动续期。
  </p>
  <el-dialog v-model="open" title="SSL 证书详情" width="650px"
    ><dl v-if="detail" class="website-details">
      <dt>域名</dt>
      <dd>{{ detail.domain }}</dd>
      <dt>证书机构</dt>
      <dd>{{ detail.issuer || "—" }}</dd>
      <dt>有效期</dt>
      <dd>{{ date(detail.not_before) }} — {{ date(detail.not_after) }}</dd>
      <dt>TLS 版本</dt>
      <dd>{{ detail.tls_version || "—" }}</dd>
      <dt>剩余天数</dt>
      <dd>{{ detail.not_after ? detail.days_left : "—" }}</dd>
      <dt>证书链</dt>
      <dd>
        <p v-for="(item, i) in JSON.parse(detail.chain_json || '[]')" :key="i">
          {{ item }}
        </p>
      </dd>
      <dt>检测时间</dt>
      <dd>{{ date(detail.updated_at) }}</dd>
    </dl></el-dialog
  >
</template>
