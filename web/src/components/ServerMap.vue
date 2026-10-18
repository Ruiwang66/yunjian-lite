<script setup>
import { computed, ref, watch } from "vue";
import { Globe, MapPin, X } from "lucide-vue-next";
import { ElMessage } from "element-plus";
import geometry from "./world-map.json";
import CountrySelect from "./CountrySelect.vue";
import { distribution } from "../geography";
import { api } from "../api";
const props = defineProps({
  servers: { type: Array, default: () => [] },
  publicView: Boolean,
  editable: Boolean,
});
const emit = defineEmits(["updated"]);
const summary = computed(() => distribution(props.servers));
const groups = computed(
  () => new Map(summary.value.groups.map((g) => [g.code, g])),
);
const selected = ref(""),
  hovered = ref(""),
  editorOpen = ref(false),
  serverId = ref(null),
  country = ref(""),
  saving = ref(false);
const active = computed(() => groups.value.get(selected.value) || null);
const hoveredGroup = computed(() => groups.value.get(hovered.value) || null);
const label = (region) =>
  `${region.name}：${groups.value.get(region.code)?.total || 0} 台服务器`;
function pick(code) {
  if (!groups.value.has(code)) return;
  selected.value = selected.value === code ? "" : code;
  hovered.value = "";
}
function openEditor() {
  serverId.value =
    props.servers.find((s) => !s.country_code)?.id ||
    props.servers[0]?.id ||
    null;
  country.value =
    props.servers.find((s) => s.id === serverId.value)?.country_code || "";
  editorOpen.value = true;
}
watch(serverId, () => {
  country.value =
    props.servers.find((s) => s.id === serverId.value)?.country_code || "";
});
watch(
  () => props.servers,
  () => {
    if (selected.value && !groups.value.has(selected.value))
      selected.value = "";
  },
);
async function save() {
  saving.value = true;
  try {
    await api.location(serverId.value, country.value);
    editorOpen.value = false;
    emit("updated");
    ElMessage.success("服务器地区已保存");
  } catch (e) {
    ElMessage.error(e.message);
  } finally {
    saving.value = false;
  }
}
</script>
<template>
  <section class="server-map panel" aria-label="服务器分布地图">
    <div class="map-heading">
      <div>
        <span class="eyebrow">GLOBAL INFRASTRUCTURE</span>
        <h2><Globe :size="19" />服务器分布</h2>
        <p v-if="summary.groups.length">
          服务器分布在 <b>{{ summary.groups.length }}</b> 个国家或地区<span
            v-if="summary.unknown"
          >
            · {{ summary.unknown }} 台未设置地区</span
          >
        </p>
        <p v-else>尚未设置服务器地区，配置后将在地图上显示。</p>
      </div>
      <button
        v-if="editable && servers.length"
        class="map-location-button"
        @click="openEditor"
      >
        <MapPin :size="14" />设置地区
      </button>
    </div>
    <div class="map-canvas">
      <svg
        :viewBox="`0 0 ${geometry.width} ${geometry.height}`"
        role="group"
        aria-label="世界服务器分布，绿色表示有在线服务器，灰绿色表示全部离线"
      >
        <path
          v-for="(path, i) in geometry.neutral"
          :key="`neutral-${i}`"
          :d="path"
          class="map-land"
          aria-hidden="true"
          fill-rule="evenodd"
        />
        <g
          v-for="region in geometry.regions"
          :key="region.code"
          :aria-hidden="!groups.has(region.code)"
        >
          <path
            v-if="region.path"
            :d="region.path"
            fill-rule="evenodd"
            class="map-land"
            :class="{
              'is-occupied': groups.has(region.code),
              'is-online': groups.get(region.code)?.online > 0,
              'is-selected': selected === region.code,
              'is-hovered': hovered === region.code,
            }"
            :role="groups.has(region.code) ? 'button' : undefined"
            :tabindex="groups.has(region.code) ? 0 : undefined"
            :aria-label="label(region)"
            :aria-pressed="
              groups.has(region.code) ? selected === region.code : undefined
            "
            @click="pick(region.code)"
            @keydown.enter.prevent="pick(region.code)"
            @keydown.space.prevent="pick(region.code)"
            @mouseenter="hovered = region.code"
            @mouseleave="hovered = ''"
            @focus="hovered = region.code"
            @blur="hovered = ''"
          >
            <title>{{ label(region) }}</title>
          </path>
          <circle
            v-if="region.tiny && groups.has(region.code)"
            :cx="region.x"
            :cy="region.y"
            r="4.5"
            class="map-marker"
            :class="{
              'is-online': groups.get(region.code)?.online > 0,
              'is-selected': selected === region.code,
            }"
            role="button"
            tabindex="0"
            :aria-label="label(region)"
            :aria-pressed="selected === region.code"
            @click="pick(region.code)"
            @keydown.enter.prevent="pick(region.code)"
            @keydown.space.prevent="pick(region.code)"
            @mouseenter="hovered = region.code"
            @mouseleave="hovered = ''"
            @focus="hovered = region.code"
            @blur="hovered = ''"
          >
            <title>{{ label(region) }}</title>
          </circle>
        </g>
      </svg>
      <div v-if="hoveredGroup" class="map-hover" aria-hidden="true">
        <strong>{{ hoveredGroup.name }}</strong
        ><span
          >{{ hoveredGroup.total }} 台 · {{ hoveredGroup.online }} 台在线</span
        >
      </div>
      <div v-if="!summary.groups.length" class="map-empty">
        <MapPin :size="20" /><span>等待添加服务器地区</span>
      </div>
    </div>
    <div class="map-bottom">
      <div class="map-legend">
        <span><i class="map-online-dot" />有在线服务器</span
        ><span><i class="map-offline-dot" />全部离线 / 待连接</span>
      </div>
      <span class="map-instruction">点击高亮地区查看服务器</span>
    </div>
    <div v-if="summary.groups.length" class="map-regions">
      <button
        v-for="group in summary.groups"
        :key="group.code"
        :class="{ active: selected === group.code }"
        :aria-pressed="selected === group.code"
        @click="pick(group.code)"
      >
        {{ group.name }}<span>{{ group.total }}</span>
      </button>
    </div>
    <div v-if="active" class="map-selection">
      <div class="map-selection-heading">
        <h3>
          {{ active.name }}<span>{{ active.total }} 台服务器</span>
        </h3>
        <button aria-label="关闭地区详情" @click="selected = ''">
          <X :size="16" />
        </button>
      </div>
      <div class="map-server-list">
        <router-link
          v-for="server in active.servers"
          :key="server.id"
          :to="
            publicView
              ? `/status/servers/${server.id}`
              : `/servers/${server.id}`
          "
          ><i class="status-dot" :class="server.status" /><span>{{
            server.name
          }}</span
          ><small>{{
            server.status === "online"
              ? "在线"
              : server.status === "offline"
                ? "离线"
                : "待连接"
          }}</small></router-link
        >
      </div>
    </div>
  </section>
  <el-dialog
    v-model="editorOpen"
    title="设置服务器地区"
    width="min(480px,92vw)"
    :close-on-click-modal="false"
    ><el-form label-position="top"
      ><el-form-item label="服务器"
        ><el-select v-model="serverId" style="width: 100%"
          ><el-option
            v-for="s in servers"
            :key="s.id"
            :label="s.name"
            :value="s.id" /></el-select></el-form-item
      ><el-form-item label="国家或地区"
        ><CountrySelect v-model="country"
      /></el-form-item>
      <p class="small muted">
        选择服务器实际所在地区。前台会公开此地区，IP
        地址保持私有；清空后归为未设置。
      </p></el-form
    ><template #footer
      ><el-button @click="editorOpen = false">取消</el-button
      ><el-button
        type="primary"
        :loading="saving"
        :disabled="!serverId"
        @click="save"
        >保存地区</el-button
      ></template
    ></el-dialog
  >
</template>
