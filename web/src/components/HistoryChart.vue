<script setup>
import { computed } from "vue";
import { date } from "../format";
const props = defineProps({ samples: { type: Array, default: () => [] } });
const width = 900,
  height = 180;
function path(key) {
  const items = props.samples;
  if (!items.length) return "";
  const start = new Date(items[0].created_at).getTime(),
    end = new Date(items.at(-1).created_at).getTime();
  return items
    .map(
      (m, i) =>
        `${i ? "L" : "M"}${end === start ? width / 2 : ((new Date(m.created_at).getTime() - start) / (end - start)) * width},${height - ((m[key] || 0) / 100) * height}`,
    )
    .join(" ");
}
const cpu = computed(() => path("cpu")),
  memory = computed(() => path("memory")),
  disk = computed(() => path("disk"));
</script>
<template>
  <div class="history-chart">
    <div v-if="samples.length < 2" class="chart-empty">
      {{
        samples.length
          ? "已接收首个样本，等待下一次采集形成趋势。"
          : "暂无历史数据，监控客户端连接后将开始记录。"
      }}
    </div>
    <template v-else
      ><div class="chart-legend">
        <span><i class="cpu-line"></i>CPU</span
        ><span><i class="memory-line"></i>内存</span
        ><span><i class="disk-line"></i>磁盘</span>
      </div>
      <div class="chart-body">
        <div class="chart-axis">
          <span>100%</span><span>50%</span><span>0%</span>
        </div>
        <svg
          :viewBox="`0 -3 ${width} ${height + 6}`"
          preserveAspectRatio="none"
          role="img"
          aria-label="CPU、内存和磁盘使用率历史曲线"
        >
          <line
            v-for="y in [0, 45, 90, 135, 180]"
            :key="y"
            x1="0"
            :x2="width"
            :y1="y"
            :y2="y"
            stroke="#e9eaed"
            stroke-dasharray="4 5"
          />
          <path
            :d="disk"
            fill="none"
            stroke="#a8acb3"
            stroke-width="2"
            vector-effect="non-scaling-stroke"
            stroke-dasharray="5 4"
          />
          <path
            :d="memory"
            fill="none"
            stroke="#69717d"
            stroke-width="2"
            vector-effect="non-scaling-stroke"
          />
          <path
            :d="cpu"
            fill="none"
            stroke="#191c21"
            stroke-width="2"
            vector-effect="non-scaling-stroke"
          />
        </svg>
      </div>
      <div class="chart-times">
        <span>{{ date(samples[0].created_at) }}</span
        ><span>{{ date(samples.at(-1).created_at) }}</span>
      </div></template
    >
  </div>
</template>
