<script setup>
import { useRoute } from "vue-router";
import { Cloud, ArrowUpRight, Map } from "lucide-vue-next";
import { live } from "../state";
import AppearanceMenu from "../components/AppearanceMenu.vue";
import Dashboard from "./Dashboard.vue";
import Server from "./Server.vue";
import { ref } from "vue";
const mapExpanded = ref(false);
const route = useRoute();
</script>
<template>
  <div class="public-shell">
    <header class="public-header glass-surface">
      <router-link to="/" class="public-brand"
        ><Cloud :size="26" />云简 <span>Lite</span></router-link
      >
      <div class="public-actions">
        <AppearanceMenu /><button
          v-if="!route.params.id"
          class="icon-button"
          aria-label="展开或收起地图"
          :aria-expanded="mapExpanded"
          aria-controls="server-distribution-map"
          title="服务器地图"
          @click="mapExpanded = !mapExpanded"
        >
          <Map :size="18" />
        </button>
        <span class="connection"
          ><i :class="{ connected: live }"></i
          >{{ live ? "实时连接" : "每 4 秒刷新" }}</span
        >
        <router-link
          to="/admin"
          class="text-link"
          aria-label="后台管理"
          title="后台管理"
          ><span class="admin-label">后台管理</span><ArrowUpRight :size="15"
        /></router-link>
      </div>
    </header>
    <main class="public-content">
      <Server v-if="route.params.id" :key="route.params.id" public-view />
      <Dashboard v-else public-view :map-expanded="mapExpanded" />
    </main>
    <footer class="footer">
      <span>CloudEase Lite · 服务器监控</span
      ><span>每 4 秒采集 · 数据保留 30 天</span>
    </footer>
  </div>
</template>
