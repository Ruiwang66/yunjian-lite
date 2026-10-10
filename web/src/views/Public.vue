<script setup>
import { useRoute } from "vue-router";
import { Cloud, ArrowUpRight } from "lucide-vue-next";
import { live } from "../state";
import Dashboard from "./Dashboard.vue";
import Server from "./Server.vue";
const route = useRoute();
</script>
<template>
  <div class="public-shell">
    <header class="public-header">
      <router-link to="/" class="public-brand"
        ><Cloud :size="26" />云简 <span>Lite</span></router-link
      >
      <div class="public-actions">
        <span class="connection"
          ><i :class="{ connected: live }"></i
          >{{ live ? "实时连接" : "每 4 秒刷新" }}</span
        >
        <router-link to="/admin" class="text-link"
          >后台管理<ArrowUpRight :size="15"
        /></router-link>
      </div>
    </header>
    <main class="public-content">
      <Server v-if="route.params.id" :key="route.params.id" public-view />
      <Dashboard v-else public-view />
    </main>
    <footer class="footer">
      <span>CloudEase Lite · 服务器监控</span
      ><span>每 4 秒采集 · 数据保留 7 天</span>
    </footer>
  </div>
</template>
