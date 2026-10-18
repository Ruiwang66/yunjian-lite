<script setup>
defineProps({
  title: String,
  description: String,
  scope: Object,
  systemOnly: Boolean,
});
</script>
<template>
  <div class="page-heading">
    <div>
      <span class="eyebrow">CLOUDEASE / V0.3</span>
      <h1>{{ title }}</h1>
      <p>{{ description }}</p>
    </div>
    <div class="management-selector">
      <el-select
        v-model="scope.selected"
        placeholder="选择服务器"
        :disabled="scope.loading"
      >
        <el-option
          v-for="s in scope.servers"
          :key="s.id"
          :label="`${s.name} · ${s.status === 'online' ? '在线' : '离线'}`"
          :value="s.id"
        />
      </el-select>
      <el-button
        :disabled="scope.busy || scope.loading"
        @click="scope.loadInfo()"
        >刷新</el-button
      >
    </div>
  </div>
  <el-alert
    v-if="scope.error"
    :title="scope.error"
    type="error"
    show-icon
    :closable="false"
    class="page-error"
  />
  <div v-if="scope.loading" class="panel muted">正在读取服务器信息…</div>
  <div v-else-if="!scope.servers.length" class="panel muted">
    请先在服务器页面添加监控客户端。
  </div>
  <el-alert
    v-else-if="scope.info && scope.info.server.status !== 'online'"
    title="此服务器当前离线，展示的状态可能已过期。"
    type="warning"
    :closable="false"
    class="page-error"
  />
  <el-alert
    v-else-if="!systemOnly && scope.info && !scope.info.capabilities.enabled"
    title="此服务器未启用运维管理。请使用 v0.2 Linux 客户端，并在安装时选择启用运维管理。"
    type="info"
    :closable="false"
    class="page-error"
  />
  <p v-if="scope.busy" class="muted" role="status">
    操作处理中，正在等待服务器返回结果…
  </p>
</template>
