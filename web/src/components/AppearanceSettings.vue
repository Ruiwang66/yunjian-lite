<script setup>
import { ref } from "vue";
import { ArrowUp, ArrowDown, Check } from "lucide-vue-next";
import {
  appearance,
  dashboard,
  updateAppearance,
  updateDashboard,
  resetAppearance,
  resetDashboard,
  storageAvailable,
} from "../appearance";
const notice = ref("");
const groups = [
  {
    key: "scheme",
    label: "明暗模式",
    items: { light: "浅色", dark: "深色", system: "跟随系统" },
  },
  {
    key: "accent",
    label: "强调色",
    items: { graphite: "石墨", blue: "海蓝", teal: "青绿", violet: "鸢紫" },
  },
  {
    key: "density",
    label: "后台信息密度",
    items: { comfortable: "舒适", compact: "紧凑" },
  },
  {
    key: "glass",
    label: "毛玻璃材质",
    items: { off: "关闭", light: "轻盈", clear: "通透" },
  },
  {
    key: "motion",
    label: "动效",
    items: { system: "跟随系统", reduced: "减少动效" },
  },
];
function reset(group) {
  if (group === "appearance") resetAppearance();
  else resetDashboard();
  notice.value =
    group === "appearance" ? "外观已恢复默认" : "概览布局已恢复默认";
}
function move(index, direction) {
  const sections = [...dashboard.sections];
  [sections[index], sections[index + direction]] = [
    sections[index + direction],
    sections[index],
  ];
  updateDashboard({ sections });
}
</script>
<template>
  <section
    class="panel appearance-settings"
    aria-labelledby="appearance-settings-title"
  >
    <div class="settings-heading">
      <div>
        <h2 id="appearance-settings-title">外观与布局</h2>
        <p class="muted small">
          外观保存在当前浏览器，公开页、登录页和后台共用。
        </p>
      </div>
      <button class="settings-reset" @click="reset('appearance')">
        恢复默认外观
      </button>
    </div>
    <p v-if="!storageAvailable" role="status" class="storage-note">
      浏览器未允许保存偏好，本次浏览有效。
    </p>
    <fieldset v-for="group in groups" :key="group.key" class="appearance-field">
      <legend>{{ group.label }}</legend>
      <div class="choice-row">
        <button
          v-for="(label, key) in group.items"
          :key="key"
          :aria-pressed="appearance[group.key] === key"
          :data-swatch="group.key === 'accent' ? key : undefined"
          @click="updateAppearance({ [group.key]: key })"
        >
          <span v-if="group.key === 'accent'" class="color-swatch"></span
          >{{ label }}<Check v-if="appearance[group.key] === key" :size="14" />
        </button>
      </div>
    </fieldset>
    <div class="appearance-preview">
      <div class="preview-checker">
        <div class="glass-surface preview-glass">材质预览</div>
      </div>
      <div class="preview-sample">
        <strong>示例监控卡</strong>
        <p><i class="status-dot online"></i>在线 <b>CPU 12.8%</b></p>
        <div>内存 <b>1.2 GB / 4 GB</b></div>
      </div>
    </div>
    <p class="muted small">
      系统要求减少动效时优先尊重系统；危险操作和在线状态保留各自颜色。
    </p>
  </section>
  <section
    class="panel appearance-settings"
    aria-labelledby="dashboard-settings-title"
  >
    <div class="settings-heading">
      <div>
        <h2 id="dashboard-settings-title">概览布局</h2>
        <p class="muted small">
          仅作用于此浏览器中的当前账户，统计固定在首位。
        </p>
      </div>
      <button class="settings-reset" @click="reset('dashboard')">
        恢复默认布局
      </button>
    </div>
    <div
      v-for="(section, index) in dashboard.sections"
      :key="section"
      class="layout-order-row"
    >
      <span>{{ section === "map" ? "服务器地图" : "最近活动" }}</span
      ><button
        class="icon-button"
        :disabled="index === 0"
        :aria-label="`上移${section === 'map' ? '地图' : '最近活动'}`"
        @click="move(index, -1)"
      >
        <ArrowUp :size="16" /></button
      ><button
        class="icon-button"
        :disabled="index === dashboard.sections.length - 1"
        :aria-label="`下移${section === 'map' ? '地图' : '最近活动'}`"
        @click="move(index, 1)"
      >
        <ArrowDown :size="16" />
      </button>
    </div>
    <label class="layout-checkbox"
      ><input
        type="checkbox"
        :checked="dashboard.mapExpanded"
        @change="updateDashboard({ mapExpanded: $event.target.checked })"
      />进入概览时展开地图</label
    >
  </section>
  <p role="status" aria-live="polite" class="settings-notice">{{ notice }}</p>
</template>
