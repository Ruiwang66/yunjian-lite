<script setup>
import { ref, watch, onUnmounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  LayoutDashboard,
  Server,
  ScrollText,
  Cloud,
  LogOut,
  KeyRound,
  Radio,
  Menu,
} from "lucide-vue-next";
import { ElMessage } from "element-plus";
import { api } from "./api";
import { user, live, startLive, stopLive } from "./state";
const route = useRoute(),
  router = useRouter(),
  menu = ref(false),
  passwordOpen = ref(false),
  saving = ref(false);
const form = ref({ current_password: "", new_password: "" });
watch(
  [() => route.path, () => route.meta.public, () => user.value?.id],
  () => {
    menu.value = false;
    // On a direct home-page refresh, auth may resolve after the initial route.
    if (route.path === "/login") stopLive();
    else if (route.meta.public) startLive(true);
    else if (user.value) startLive();
    else stopLive();
  },
  { immediate: true },
);
async function logout() {
  try {
    await api.logout();
    user.value = null;
    stopLive();
    router.push("/login");
  } catch (e) {
    ElMessage.error(e.message);
  }
}
async function changePassword() {
  saving.value = true;
  try {
    await api.password(form.value);
    form.value = { current_password: "", new_password: "" };
    passwordOpen.value = false;
    user.value = null;
    stopLive();
    ElMessage.success("密码已更新，请重新登录");
    router.push("/login");
  } catch (e) {
    ElMessage.error(e.message);
  } finally {
    saving.value = false;
  }
}
function expired() {
  user.value = null;
  stopLive();
  if (!route.meta.public) router.replace("/login");
}
window.addEventListener("session-expired", expired);
onUnmounted(() => {
  window.removeEventListener("session-expired", expired);
  stopLive();
});
</script>
<template>
  <router-view v-if="route.meta.public" />
  <div v-else class="app-shell">
    <button class="mobile-menu" aria-label="打开导航" @click="menu = !menu">
      <Menu :size="20" />
    </button>
    <div v-if="menu" class="menu-shade" @click="menu = false"></div>
    <aside class="sidebar" :class="{ 'is-open': menu }">
      <router-link to="/admin" class="brand"
        ><span class="brand-mark"><Cloud :size="23" :stroke-width="1.5" /></span
        ><span
          >云简<span class="brand-lite"> Lite</span
          ><small>CLOUDEASE</small></span
        ></router-link
      >
      <div class="nav-label">工作空间</div>
      <nav>
        <router-link to="/admin" :class="{ active: route.path === '/admin' }"
          ><LayoutDashboard :size="18" />概览<span class="nav-short"
            >01</span
          ></router-link
        >
        <router-link
          to="/servers"
          :class="{ active: route.path.startsWith('/servers') }"
          ><Server :size="18" />服务器<span class="nav-short"
            >02</span
          ></router-link
        >
        <router-link to="/logs" :class="{ active: route.path === '/logs' }"
          ><ScrollText :size="18" />系统日志<span class="nav-short"
            >03</span
          ></router-link
        >
      </nav>
      <div class="sidebar-foot">
        <div class="release-card">
          <Radio :size="17" /><span
            >轻量，始终在线。<small>每 4 秒采集一次</small></span
          >
        </div>
        <div class="version-row">
          <span>v0.1.0 · MVP</span><span>开源 / MIT</span>
        </div>
      </div>
    </aside>
    <div class="main-shell">
      <header class="topbar">
        <span class="breadcrumb"
          >工作空间 <span>/</span>
          {{
            route.path === "/admin"
              ? "概览"
              : route.path === "/logs"
                ? "系统日志"
                : "服务器"
          }}</span
        >
        <div class="top-actions">
          <router-link to="/" class="text-link">前台监控</router-link>
          <span class="connection"
            ><i :class="{ connected: live }"></i
            >{{ live ? "实时连接" : "轮询刷新" }}</span
          ><el-dropdown trigger="click"
            ><button class="user-button">
              <span class="avatar">A</span><span>admin</span></button
            ><template #dropdown
              ><el-dropdown-menu
                ><el-dropdown-item @click="passwordOpen = true"
                  ><KeyRound
                    :size="14"
                    class="inline-icon"
                  />修改密码</el-dropdown-item
                ><el-dropdown-item @click="logout"
                  ><LogOut
                    :size="14"
                    class="inline-icon"
                  />退出登录</el-dropdown-item
                ></el-dropdown-menu
              ></template
            ></el-dropdown
          >
        </div>
      </header>
      <main class="content"><router-view /></main>
      <footer class="footer">
        <span
          >CloudEase Lite
          <span class="footer-dot">·</span> 让服务器管理更简单</span
        ><span>轻量监控 / 自主掌控</span>
      </footer>
    </div>
    <el-dialog
      v-model="passwordOpen"
      title="修改管理员密码"
      width="min(440px, 92vw)"
      @closed="form = { current_password: '', new_password: '' }"
      ><el-form label-position="top" @submit.prevent="changePassword"
        ><el-form-item label="当前密码"
          ><el-input
            v-model="form.current_password"
            type="password"
            show-password
            autocomplete="current-password" /></el-form-item
        ><el-form-item label="新密码"
          ><el-input
            v-model="form.new_password"
            type="password"
            show-password
            autocomplete="new-password"
            minlength="12"
            maxlength="72"
        /></el-form-item>
        <p class="muted small">12–72 字节。修改后所有登录会话将失效。</p>
        <el-button type="primary" native-type="submit" :loading="saving"
          >保存新密码</el-button
        ></el-form
      ></el-dialog
    >
  </div>
</template>
