<script setup>
import { ref, watch } from "vue";
import ManagementHeader from "../components/ManagementHeader.vue";
import { useManagement } from "../management";
import { uptime } from "../format";
const scope = useManagement(),
  data = ref(null);
watch(
  () => scope.info,
  async (value) => {
    data.value = null;
    if (!value) return;
    try {
      data.value = await scope.execute("/system/info");
    } catch {}
  },
);
</script>
<template>
  <ManagementHeader
    title="系统信息"
    description="查看硬件、系统版本、网络地址和虚拟化信息。"
    :scope="scope"
    system-only
  />
  <div v-if="data" class="panel management-panel">
    <dl class="system-grid">
      <div>
        <dt>CPU 型号</dt>
        <dd>{{ data.system.cpu_model || "请升级监控客户端" }}</dd>
      </div>
      <div>
        <dt>逻辑核心数</dt>
        <dd>{{ data.server.cpu_cores || "—" }}</dd>
      </div>
      <div>
        <dt>系统发行版</dt>
        <dd>{{ data.server.platform }} {{ data.server.platform_version }}</dd>
      </div>
      <div>
        <dt>内核版本</dt>
        <dd>{{ data.system.kernel_version || "—" }}</dd>
      </div>
      <div>
        <dt>架构</dt>
        <dd>{{ data.server.arch || "—" }}</dd>
      </div>
      <div>
        <dt>虚拟化类型</dt>
        <dd>{{ data.system.virtualization || "未检测到" }}</dd>
      </div>
      <div>
        <dt>公网 IP</dt>
        <dd>{{ data.system.public_ips?.join(" / ") || "未检测到公网地址" }}</dd>
      </div>
      <div>
        <dt>内网 IP</dt>
        <dd>{{ data.system.private_ips?.join(" / ") || "—" }}</dd>
      </div>
      <div>
        <dt>运行时间</dt>
        <dd>{{ uptime(data.server.uptime) }}</dd>
      </div>
      <div>
        <dt>监控客户端版本</dt>
        <dd>{{ scope.info.capabilities.version || "v0.1" }}</dd>
      </div>
    </dl>
    <p class="small muted">
      公网地址来自本机网卡及控制台观察到的上报来源，经过代理或 NAT
      时可能显示出口地址。
    </p>
  </div>
</template>
