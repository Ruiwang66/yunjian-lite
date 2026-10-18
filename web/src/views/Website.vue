<script setup>
import { ref, computed, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import {
  Globe,
  Plus,
  ShieldCheck,
  Database,
  Activity,
  ExternalLink,
  Code,
  Archive,
  RefreshCw,
} from "lucide-vue-next";
import ManagementHeader from "../components/ManagementHeader.vue";
import { useManagement } from "../management";
import { request } from "../api";
const scope = useManagement(),
  sites = ref([]),
  inventory = ref(null),
  backups = ref([]);
const editorOpen = ref(false),
  detailsOpen = ref(false),
  saving = ref(false),
  editing = ref(null),
  tab = ref("health"),
  details = ref(null),
  config = ref(null);
const form = ref({ name: "", domain: "", root_path: "", nginx_config: "" });
const ready = computed(
  () =>
    scope.info?.server.status === "online" &&
    scope.info?.capabilities.websites?.enabled,
);
const checks = computed(
  () => new Map((inventory.value?.checks || []).map((c) => [c.domain, c])),
);
const activeSites = computed(
  () =>
    sites.value.filter(
      (s) => (checks.value.get(s.domain)?.status || s.status) === "正常",
    ).length,
);
const options = computed(() => inventory.value?.sites || []);
async function load() {
  sites.value = [];
  inventory.value = null;
  backups.value = [];
  detailsOpen.value = false;
  if (!scope.selected || !scope.info) return;
  const selected = scope.selected;
  try {
    const [rows, records] = await Promise.all([
      request(`/websites?server_id=${selected}`),
      request(`/backup?server_id=${selected}`),
    ]);
    if (selected !== scope.selected) return;
    sites.value = rows.items;
    inventory.value = rows.inventory;
    backups.value = records.items;
  } catch (e) {
    if (selected === scope.selected) scope.error = e.message;
  }
}
watch(() => scope.info, load);
function selectDomain(domain) {
  const item = options.value.find((s) => s.domain === domain);
  if (item) form.value = { ...item, name: form.value.name || item.name };
}
function edit(site) {
  editing.value = site?.id || null;
  form.value = site
    ? {
        name: site.name,
        domain: site.domain,
        root_path: site.root_path,
        nginx_config: site.nginx_config,
      }
    : { name: "", domain: "", root_path: "", nginx_config: "" };
  editorOpen.value = true;
}
async function save() {
  saving.value = true;
  try {
    await request(editing.value ? `/websites/${editing.value}` : "/websites", {
      method: editing.value ? "PATCH" : "POST",
      body: JSON.stringify({ ...form.value, server_id: scope.selected }),
    });
    editorOpen.value = false;
    ElMessage.success("网站记录已保存");
    await load();
  } catch (e) {
    ElMessage.error(e.message);
  } finally {
    saving.value = false;
  }
}
async function remove(site) {
  try {
    await ElMessageBox.confirm(
      `删除「${site.name}」的控制台记录？主机文件、Nginx 配置和已生成的备份会保留。`,
      "删除网站",
      { confirmButtonText: "删除记录", cancelButtonText: "取消" },
    );
    await request(`/websites/${site.id}`, { method: "DELETE" });
    await load();
    ElMessage.success("网站记录已删除");
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message);
  }
}
async function inspect(site) {
  try {
    details.value = await request(`/websites/${site.id}`);
    config.value = null;
    tab.value = "health";
    detailsOpen.value = true;
  } catch (e) {
    ElMessage.error(e.message);
  }
}
async function detect(site) {
  try {
    await scope.execute(
      `/websites/${site.id}/check`,
      { website_id: site.id },
      "POST",
    );
    ElMessage.success("检测完成");
    await scope.loadInfo();
  } catch {}
}
async function readConfig() {
  try {
    config.value = await scope.execute("/nginx/site/config", {
      website_id: details.value.website.id,
    });
  } catch {}
}
async function toggleSite(enable) {
  try {
    const w = details.value.website;
    await ElMessageBox.confirm(
      `确认${enable ? "启用" : "禁用"} ${w.domain}？会自动备份配置、执行 nginx -t 并重载 Nginx。禁用后该站点可能无法访问。`,
      "Nginx 站点操作",
      { confirmButtonText: "确认执行", cancelButtonText: "取消" },
    );
    config.value = await scope.execute(
      `/nginx/site/${enable ? "enable" : "disable"}`,
      { website_id: w.id },
      "POST",
    );
    ElMessage.success("站点操作完成");
  } catch {}
}
async function backup(site, kind) {
  try {
    await ElMessageBox.confirm(
      `为 ${site.domain} 创建${kind === "files" ? "网站文件" : "数据库"}备份？备份保存在目标主机的私有目录，最多 256 MiB。`,
      "手动备份",
      { confirmButtonText: "开始备份", cancelButtonText: "取消" },
    );
    await scope.execute("/backup", { website_id: site.id, kind }, "POST");
    ElMessage.success("备份完成");
  } catch {
  } finally {
    try {
      backups.value = (
        await request(`/backup?server_id=${scope.selected}`)
      ).items;
    } catch {}
  }
}
async function inspectPHP() {
  try {
    const php = await scope.execute("/php/inspect", {}, "POST");
    if (inventory.value) inventory.value.php = php;
    ElMessage.success("PHP 环境检测完成");
  } catch {}
}
const date = (value) => (value ? new Date(value).toLocaleString() : "—");
const size = (value) =>
  value >= 1048576
    ? `${(value / 1048576).toFixed(1)} MB`
    : `${(value / 1024).toFixed(1)} KB`;
</script>
<template>
  <ManagementHeader
    title="网站与应用"
    description="管理站点、检查访问状态，安全地维护 Web 服务与备份。"
    :scope="scope"
  />
  <el-alert
    v-if="scope.info && !ready"
    title="网站管理需要 v0.3 Linux 客户端和主机本地网站白名单。配置步骤见项目 docs/V0.3.md。目前只支持标准 Nginx 目录，尚未适配宝塔。"
    type="info"
    show-icon
    :closable="false"
    class="page-error"
  />
  <template v-if="scope.info">
    <el-alert
      v-if="inventory?.demo"
      title="本地演示：Linux 环境、访问检测与 Nginx 操作为模拟数据；备份仅写入独立测试目录，不会操作真实服务器。"
      type="warning"
      :closable="false"
      show-icon
      class="page-error"
    />
    <div class="website-summary">
      <div class="website-summary-card">
        <Globe :size="20" /><small>已登记网站</small
        ><strong
          >{{ sites.length
          }}<span> / {{ options.length }} 个授权站点</span></strong
        >
      </div>
      <div class="website-summary-card">
        <Activity :size="20" /><small>访问正常</small
        ><strong>{{ activeSites }}<span>HTTPS 与响应状态</span></strong>
      </div>
      <div class="website-summary-card">
        <Code :size="20" /><small>Web 环境</small
        ><strong
          >{{ inventory?.nginx.available ? "Nginx" : "未检测"
          }}<span>{{ inventory?.nginx.state || "—" }}</span></strong
        >
      </div>
      <div class="website-summary-card">
        <Archive :size="20" /><small>手动备份</small
        ><strong
          >{{ backups.filter((b) => b.status === "success").length
          }}<span>自动备份未开启</span></strong
        >
      </div>
    </div>
    <div class="section-heading website-heading">
      <div>
        <h2>你的网站</h2>
        <p class="muted small">
          登记已授权站点，不会自动创建目录或覆盖主机配置。
        </p>
      </div>
      <el-button
        type="primary"
        :disabled="!ready || !scope.canOperate"
        @click="edit()"
        ><Plus :size="16" />添加网站</el-button
      >
    </div>
    <div class="website-grid" v-if="sites.length">
      <article v-for="site in sites" :key="site.id" class="website-card">
        <div class="website-card-top">
          <div class="website-symbol"><Globe :size="22" /></div>
          <span
            class="website-status"
            :class="{
              healthy:
                (checks.get(site.domain)?.status || site.status) === '正常',
            }"
            ><i />{{ checks.get(site.domain)?.status || site.status }}</span
          >
        </div>
        <h3>{{ site.name }}</h3>
        <a
          :href="`https://${site.domain}`"
          target="_blank"
          rel="noopener noreferrer"
          class="website-domain"
          >{{ site.domain }}<ExternalLink :size="13"
        /></a>
        <div class="website-card-facts">
          <span>Web 服务<b>Nginx</b></span
          ><span
            >SSL<b>{{
              checks.get(site.domain)?.tls.status || "未检测"
            }}</b></span
          ><span
            >PHP<b>{{
              inventory?.php.available
                ? inventory.php.version.replace(/^PHP /, "").split(" ")[0]
                : "未安装"
            }}</b></span
          ><span
            >响应<b
              >{{ checks.get(site.domain)?.response_ms ?? "—" }} ms</b
            ></span
          >
        </div>
        <div
          v-if="checks.get(site.domain)?.tls.warning"
          class="website-warning"
        >
          {{ checks.get(site.domain).tls.warning }}
        </div>
        <div class="website-card-actions">
          <el-button @click="inspect(site)">查看</el-button
          ><el-button
            :disabled="!ready || scope.busy || !scope.canOperate"
            @click="detect(site)"
            >检测</el-button
          ><el-dropdown v-if="scope.canOperate" trigger="click"
            ><el-button :disabled="scope.busy">管理 ···</el-button
            ><template #dropdown
              ><el-dropdown-menu
                ><el-dropdown-item @click="edit(site)"
                  >编辑记录</el-dropdown-item
                ><el-dropdown-item
                  :disabled="!ready"
                  @click="backup(site, 'files')"
                  >备份网站文件</el-dropdown-item
                ><el-dropdown-item
                  :disabled="!ready"
                  @click="backup(site, 'database')"
                  >备份数据库</el-dropdown-item
                ><el-dropdown-item divided @click="remove(site)"
                  >删除记录</el-dropdown-item
                ></el-dropdown-menu
              ></template
            ></el-dropdown
          >
        </div>
      </article>
    </div>
    <div v-else class="panel website-empty">
      <Globe :size="38" />
      <h3>把第一个网站加入工作空间</h3>
      <p>先为 Linux 客户端配置站点白名单，再选择授权站点进行登记。</p>
      <el-button
        type="primary"
        :disabled="!ready || !scope.canOperate"
        @click="edit()"
        >添加网站</el-button
      >
    </div>
    <el-tabs class="website-bottom-tabs">
      <el-tab-pane label="备份记录"
        ><div class="panel">
          <div class="section-heading">
            <h2>安全留存，按需备份</h2>
            <span class="muted small">文件 ZIP · SQLite 快照 · MySQL 导出</span>
          </div>
          <el-table
            :data="backups"
            empty-text="尚无备份。网站卡片的管理菜单中可手动创建。"
            ><el-table-column
              prop="name"
              label="名称"
              min-width="230"
            /><el-table-column label="类型" width="100"
              ><template #default="{ row }">{{
                row.kind === "files" ? "网站文件" : "数据库"
              }}</template></el-table-column
            ><el-table-column label="大小" width="110"
              ><template #default="{ row }">{{
                size(row.size)
              }}</template></el-table-column
            ><el-table-column
              prop="status"
              label="状态"
              width="110"
            /><el-table-column label="创建时间" min-width="180"
              ><template #default="{ row }">{{
                date(row.created_at)
              }}</template></el-table-column
            ></el-table
          >
        </div></el-tab-pane
      >
      <el-tab-pane label="PHP 环境"
        ><div class="panel">
          <div class="section-heading">
            <h2>{{ inventory?.php.version || "未检测到 PHP" }}</h2>
            <el-button
              :disabled="!ready || !scope.canOperate || scope.busy"
              @click="inspectPHP"
              >重新检测</el-button
            >
          </div>
          <p>PHP-FPM：{{ inventory?.php.fpm_state || "—" }}</p>
          <div class="website-extensions">
            <span
              v-for="name in ['curl', 'gd', 'mysqli', 'pdo', 'openssl']"
              :key="name"
              :class="{ present: inventory?.php.extensions.includes(name) }"
              >{{ name }} ·
              {{
                inventory?.php.extensions.includes(name) ? "已安装" : "未检测到"
              }}</span
            >
          </div>
          <p
            v-for="warning in inventory?.php.warnings"
            :key="warning"
            class="website-warning"
          >
            {{ warning }}
          </p>
          <p class="small muted">
            CLI 与 FPM 可能使用不同配置；此处不会自动安装扩展或修改 php.ini。
          </p>
        </div></el-tab-pane
      >
    </el-tabs>
  </template>
  <el-dialog
    v-model="editorOpen"
    :title="editing ? '编辑网站记录' : '添加网站'"
    width="560px"
    :close-on-click-modal="false"
    ><el-form label-position="top" @submit.prevent="save"
      ><el-form-item label="网站名称"
        ><el-input
          v-model="form.name"
          maxlength="120"
          placeholder="我的博客" /></el-form-item
      ><el-form-item label="主机授权站点"
        ><el-select
          v-model="form.domain"
          placeholder="选择客户端白名单中的站点"
          style="width: 100%"
          @change="selectDomain"
          ><el-option
            v-for="site in options"
            :key="site.domain"
            :label="site.domain"
            :value="site.domain" /></el-select></el-form-item
      ><el-form-item label="网站目录"
        ><el-input :model-value="form.root_path" readonly /></el-form-item
      ><el-form-item label="Nginx 配置文件"
        ><el-input :model-value="form.nginx_config" readonly
      /></el-form-item>
      <p class="small muted">
        目录与配置由主机管理员授权，网页无法扩大范围。
      </p></el-form
    ><template #footer
      ><el-button @click="editorOpen = false">取消</el-button
      ><el-button
        type="primary"
        :loading="saving"
        :disabled="!form.name || !form.domain"
        @click="save"
        >保存网站</el-button
      ></template
    ></el-dialog
  >
  <el-dialog v-model="detailsOpen" :title="details?.website.name" width="800px"
    ><template v-if="details"
      ><el-tabs v-model="tab"
        ><el-tab-pane label="健康与域名" name="health"
          ><dl class="website-details">
            <dt>域名</dt>
            <dd>{{ details.website.domain }}</dd>
            <dt>访问状态</dt>
            <dd>
              {{ details.health.status || "未检测" }} · HTTP
              {{ details.health.http_status || "—" }} ·
              {{ details.health.response_ms || 0 }} ms
            </dd>
            <dt>A 记录</dt>
            <dd>
              {{ JSON.parse(details.domain.a_json || "[]").join(" / ") || "—" }}
            </dd>
            <dt>AAAA 记录</dt>
            <dd>
              {{
                JSON.parse(details.domain.aaaa_json || "[]").join(" / ") || "—"
              }}
            </dd>
            <dt>SSL</dt>
            <dd>
              {{ details.ssl.status || "未检测" }} ·
              {{ details.ssl.issuer || "—" }} · 剩余
              {{ details.ssl.days_left ?? "—" }} 天
            </dd>
            <dt>TLS</dt>
            <dd>{{ details.ssl.tls_version || "—" }}</dd>
            <dt>证书有效期</dt>
            <dd>
              {{ date(details.ssl.not_before) }} —
              {{ date(details.ssl.not_after) }}
            </dd>
            <dt>检测时间</dt>
            <dd>{{ date(details.health.updated_at) }}</dd>
          </dl>
          <p v-if="details.health.error" class="website-warning">
            {{ details.health.error }}
          </p>
          <p
            v-for="warning in JSON.parse(details.health.warnings_json || '[]')"
            :key="warning"
            class="website-warning"
          >
            {{ warning }}
          </p></el-tab-pane
        ><el-tab-pane label="Nginx 配置" name="nginx"
          ><div class="website-config-actions">
            <el-button :disabled="!ready || scope.busy" @click="readConfig"
              >读取配置</el-button
            ><el-button
              :disabled="!ready || scope.busy || !scope.canOperate"
              @click="toggleSite(true)"
              >启用站点</el-button
            ><el-button
              :disabled="!ready || scope.busy || !scope.canOperate"
              @click="toggleSite(false)"
              >禁用站点</el-button
            >
          </div>
          <pre v-if="config?.config" class="website-config">{{
            config.config
          }}</pre>
          <p v-if="config">
            当前{{ config.enabled ? "已启用" : "未启用"
            }}<span v-if="config.backup"> · 自动备份：{{ config.backup }}</span>
          </p>
          <p
            v-for="warning in config?.warnings"
            :key="warning"
            class="website-warning"
          >
            {{ warning }}
          </p>
          <p class="small muted">
            只允许查看主机授权配置与启停现有站点；不提供任意 Nginx
            命令或配置文本写入。
          </p></el-tab-pane
        ></el-tabs
      ></template
    ></el-dialog
  >
</template>
