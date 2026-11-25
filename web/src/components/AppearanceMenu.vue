<script setup>
import { ref, useId, nextTick, onMounted, onUnmounted } from "vue";
import { Sun, Moon, Monitor, X } from "lucide-vue-next";
import { appearance, updateAppearance } from "../appearance";
const open = ref(false),
  trigger = ref(null),
  panel = ref(null),
  root = ref(null),
  id = useId();
async function toggle() {
  if (open.value) return close();
  document.dispatchEvent(new Event("appearance-opening"));
  open.value = true;
  await nextTick();
  panel.value?.querySelector("button")?.focus();
}
function close() {
  if (!open.value) return;
  open.value = false;
  trigger.value?.focus();
}
function outside(e) {
  if (!root.value?.contains(e.target)) open.value = false;
}
function keys(e) {
  if (e.key === "Escape") {
    e.preventDefault();
    close();
  }
  if (e.key === "Tab") {
    const items = [...panel.value.querySelectorAll("button,a")];
    const first = items[0],
      last = items.at(-1);
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault();
      first.focus();
    }
  }
}
function other() {
  open.value = false;
}
onMounted(() => {
  document.addEventListener("pointerdown", outside);
  document.addEventListener("navigation-opening", other);
});
onUnmounted(() => {
  document.removeEventListener("pointerdown", outside);
  document.removeEventListener("navigation-opening", other);
});
</script>
<template>
  <div ref="root" class="appearance-control">
    <button
      ref="trigger"
      class="icon-button appearance-trigger"
      aria-label="外观设置"
      title="外观设置"
      :aria-expanded="open"
      :aria-controls="id"
      @click="toggle"
    >
      <Moon v-if="appearance.scheme === 'dark'" :size="18" /><Sun
        v-else
        :size="18"
      />
    </button>
    <section
      v-if="open"
      :id="id"
      ref="panel"
      class="appearance-menu glass-surface"
      role="dialog"
      aria-label="外观设置"
      @keydown="keys"
    >
      <div class="appearance-menu-head">
        <strong>外观</strong
        ><button class="icon-button" aria-label="关闭外观设置" @click="close">
          <X :size="16" />
        </button>
      </div>
      <div class="scheme-options">
        <button
          v-for="item in [
            { key: 'light', label: '浅色', icon: Sun },
            { key: 'dark', label: '深色', icon: Moon },
            { key: 'system', label: '跟随系统', icon: Monitor },
          ]"
          :key="item.key"
          :aria-pressed="appearance.scheme === item.key"
          @click="updateAppearance({ scheme: item.key })"
        >
          <component :is="item.icon" :size="17" />{{ item.label }}
        </button>
      </div>
      <p class="menu-label">材质</p>
      <div class="choice-row">
        <button
          v-for="(label, key) in { off: '关闭', light: '轻盈', clear: '通透' }"
          :key="key"
          :aria-pressed="appearance.glass === key"
          @click="updateAppearance({ glass: key })"
        >
          {{ label }}
        </button>
      </div>
      <p class="muted small">外观保存在当前浏览器</p>
    </section>
  </div>
</template>
