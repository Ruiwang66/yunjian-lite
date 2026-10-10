<script setup>
import { computed } from "vue";
import { percent } from "../format";
const props = defineProps({ label: String, value: Number, caption: String });
const width = computed(() => Math.min(100, Math.max(0, props.value || 0)));
</script>
<template>
  <div class="metric-bar">
    <div class="metric-bar-head">
      <span>{{ label }}</span
      ><span class="metric-number"
        >{{ percent(value) }}<small v-if="value != null">%</small></span
      >
    </div>
    <div class="meter">
      <span
        :style="{ width: width + '%' }"
        :class="{ hot: width >= 90 }"
      ></span>
    </div>
    <p v-if="caption" class="metric-caption">{{ caption }}</p>
  </div>
</template>
