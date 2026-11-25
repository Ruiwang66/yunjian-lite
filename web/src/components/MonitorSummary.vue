<script setup>
import { computed, ref, watch } from "vue";
import { Clock3, Server, Globe, Network, Activity } from "lucide-vue-next";
import { distribution } from "../geography";
import { bytes } from "../format";
import {
  sumField,
  sampleSignature,
  appendRatePoint,
  ratePath,
} from "../monitor-data";
const props = defineProps({ servers: Array, now: Number, loading: Boolean });
const online = computed(
  () => props.servers.filter((s) => s.status === "online").length,
);
const regions = computed(() => distribution(props.servers).groups.length);
const tx = computed(() => sumField(props.servers, "network_tx")),
  rx = computed(() => sumField(props.servers, "network_rx"));
const up = computed(() => sumField(props.servers, "network_tx_rate", true)),
  down = computed(() => sumField(props.servers, "network_rx_rate", true));
const points = ref([]);
watch(
  () => sampleSignature(props.servers),
  (signature) => {
    if (!props.loading)
      points.value = appendRatePoint(
        points.value,
        signature,
        props.servers,
        Date.now(),
      );
  },
  { immediate: true },
);
const paths = computed(() => ({
  tx: ratePath(points.value, "tx", props.now),
  rx: ratePath(points.value, "rx", props.now),
}));
const format = (v, rate = false) => (v === null ? "—" : bytes(v, rate));
</script>
<template>
  <section class="monitor-summary" aria-label="监控总览">
    <div class="summary-cell">
      <Clock3 :size="17" />
      <div>
        <span>当前时间</span
        ><strong>{{
          new Date(now).toLocaleTimeString("zh-CN", { hour12: false })
        }}</strong>
      </div>
    </div>
    <div class="summary-cell">
      <Server :size="17" />
      <div>
        <span>当前在线</span
        ><strong data-testid="online-count">{{
          loading ? "—" : `${online}/${servers.length}`
        }}</strong>
      </div>
    </div>
    <div class="summary-cell">
      <Globe :size="17" />
      <div>
        <span>点亮地区</span><strong>{{ loading ? "—" : regions }}</strong>
      </div>
    </div>
    <div
      class="summary-cell"
      title="各节点本次开机以来的累计值；节点重启可能下降，不代表月流量"
    >
      <Network :size="17" />
      <div>
        <span>累计流量</span
        ><strong class="summary-traffic"
          >↑ {{ format(tx.value) }}<br />↓ {{ format(rx.value) }}</strong
        ><small v-if="tx.covered < tx.total || rx.covered < rx.total"
          >覆盖 ↑{{ tx.covered }} ↓{{ rx.covered }} /
          {{ servers.length }} 台</small
        ><small v-else>本次开机以来</small>
      </div>
    </div>
    <div class="summary-cell summary-speed">
      <span><Activity :size="16" />网络速率 · 最近 60 秒</span
      ><svg
        viewBox="0 0 300 50"
        preserveAspectRatio="none"
        role="img"
        aria-label="在线服务器上下行速率趋势"
      >
        <path v-if="points.length > 1" :d="paths.tx" class="rate-up" />
        <path v-if="points.length > 1" :d="paths.rx" class="rate-down" />
      </svg>
      <div>
        <b>↑ {{ format(up.value, true) }}</b
        ><b>↓ {{ format(down.value, true) }}</b>
      </div>
      <small v-if="up.covered < up.total || down.covered < down.total"
        >速率覆盖 ↑{{ up.covered }} ↓{{ down.covered }} /
        {{ up.total }} 台在线</small
      >
    </div>
  </section>
</template>
