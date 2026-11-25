<script setup>
import { computed } from "vue";
import {
  Server,
  Globe,
  ArrowUp,
  ArrowDown,
  Clock3,
  Activity,
} from "lucide-vue-next";
import MetricBar from "./MetricBar.vue";
import { countryName, countries } from "../geography";
import { bytes, uptime, ago, statusText } from "../format";
import { numeric } from "../monitor-data";
const props = defineProps({ server: Object, publicView: Boolean, now: Number });
const region = computed(() =>
  countries.some((c) => c.code === props.server.country_code)
    ? props.server.country_code
    : "",
);
const flagUrl = computed(() =>
  region.value ? `/assets/flags/${region.value.toLowerCase()}.svg` : "",
);
// Some names already include a region flag; show it once.
const name = computed(() =>
  props.server.name.replace(/[\u{1F1E6}-\u{1F1FF}]{2}/gu, "").trim(),
);
const size = (value, rate = false) =>
  numeric(value) ? bytes(value, rate) : "—";
const capacity = (used, total) => `${size(used)} / ${size(total)}`;
</script>
<template>
  <router-link
    :to="(publicView ? '/status/servers/' : '/servers/') + server.id"
    class="node-card"
    :class="server.status"
    :aria-label="`${server.name}，${statusText[server.status] || '未知状态'}，查看详情`"
  >
    <div class="node-identity">
      <div>
        <h3>
          <span class="node-title-symbol">
            <img
              v-if="flagUrl"
              :src="flagUrl"
              class="region-flag"
              :alt="countryName(region)"
              :title="countryName(region)"
              width="20"
              height="15"
            /><Globe v-else :size="17" /> </span
          ><span>{{ name }}</span>
        </h3>
        <p>
          <i class="status-dot" :class="server.status"></i
          >{{ server.platform || server.os || "等待连接"
          }}<span v-if="server.arch"> · {{ server.arch }}</span>
        </p>
        <small class="node-region">{{ countryName(region) }}</small>
      </div>
      <Server
        class="node-device"
        :size="96"
        :stroke-width="1.8"
        aria-hidden="true"
      />
    </div>
    <div class="node-metrics">
      <div class="node-metric">
        <MetricBar
          label="CPU"
          :value="server.latest?.cpu"
          :caption="server.cpu_cores ? `${server.cpu_cores} Cores` : '—'"
        />
      </div>
      <div class="node-metric">
        <MetricBar
          label="RAM"
          :value="server.latest?.memory"
          :caption="
            capacity(server.latest?.memory_used, server.latest?.memory_total)
          "
        />
      </div>
      <div class="node-metric node-disk">
        <MetricBar
          label="Disk"
          :value="server.latest?.disk"
          :caption="
            capacity(server.latest?.disk_used, server.latest?.disk_total)
          "
        />
        <div class="node-traffic">
          <span><Activity :size="13" />Traffic</span>
          <p>
            ↑ {{ size(server.latest?.network_tx) }}<br />↓
            {{ size(server.latest?.network_rx) }}
          </p>
        </div>
      </div>
      <div class="node-metric node-speed">
        <span class="node-metric-label"><Activity :size="14" />Speed</span>
        <div class="speed-up">
          <span><ArrowUp :size="13" />Up</span
          ><b>{{ size(server.latest?.network_tx_rate, true) }}</b>
        </div>
        <div class="speed-down">
          <span><ArrowDown :size="13" />Down</span
          ><b>{{ size(server.latest?.network_rx_rate, true) }}</b>
        </div>
      </div>
    </div>
    <div class="node-footer">
      <span
        ><Clock3 :size="13" />{{
          numeric(server.uptime) ? uptime(server.uptime) : "—"
        }}</span
      ><span class="node-status"
        ><i class="status-dot" :class="server.status"></i
        >{{ statusText[server.status] || "未知状态" }}</span
      >
    </div>
    <p v-if="server.status !== 'online'" class="node-last">
      最后上报：{{ server.last_seen ? ago(server.last_seen, now) : "尚未连接" }}
    </p>
  </router-link>
</template>
