<script setup>
import { ref } from "vue";
import { useRouter } from "vue-router";
import {
  Cloud,
  ArrowRight,
  Activity,
  LockKeyhole,
  Server,
} from "lucide-vue-next";
import { api } from "../api";
import { user } from "../state";
const router = useRouter(),
  form = ref({ username: "admin", password: "" }),
  loading = ref(false),
  error = ref("");
async function submit() {
  if (!form.value.username || !form.value.password) {
    error.value = "请输入账号和密码";
    return;
  }
  loading.value = true;
  error.value = "";
  try {
    const result = await api.login(form.value);
    user.value = result.user;
    router.push("/admin");
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}
</script>
<template>
  <div class="login-page">
    <section class="login-story">
      <div class="login-brand"><Cloud :size="28" /> 云简 Lite</div>
      <div class="login-intro">
        <span class="eyebrow light">YOUR INFRASTRUCTURE. SIMPLIFIED.</span>
        <h1>洞察每一次波动。<br />掌控每一台服务器。</h1>
        <p>轻量的服务器监控工作空间。<br />让复杂的基础设施，回归简单。</p>
        <div class="login-features">
          <span><Activity :size="17" />实时指标</span
          ><span><Server :size="17" />多端采集</span
          ><span><LockKeyhole :size="17" />私有部署</span>
        </div>
      </div>
      <div class="login-story-foot">
        CLOUDEASE LITE <span>v0.1.0 / OPEN SOURCE</span>
      </div>
    </section>
    <section class="login-panel">
      <div class="login-form">
        <span class="eyebrow">WELCOME BACK</span>
        <h2>登录工作空间</h2>
        <router-link to="/" class="text-link">返回前台监控</router-link>
        <p class="muted">你的服务器，一目了然。</p>
        <el-form label-position="top" @submit.prevent="submit"
          ><el-form-item label="管理员账号"
            ><el-input
              v-model="form.username"
              autocomplete="username"
              size="large"
              placeholder="admin"
              maxlength="64" /></el-form-item
          ><el-form-item label="密码"
            ><el-input
              v-model="form.password"
              autocomplete="current-password"
              size="large"
              placeholder="输入管理员密码"
              type="password"
              show-password
              maxlength="72" /></el-form-item
          ><el-alert
            v-if="error"
            :title="error"
            type="error"
            :closable="false"
            show-icon
            class="login-error" /><el-button
            native-type="submit"
            type="primary"
            size="large"
            :loading="loading"
            class="login-submit"
            >登录控制台 <ArrowRight :size="17" /></el-button
        ></el-form>
        <div class="login-hint">
          <LockKeyhole :size="15" /><span
            >首次密码由服务端自动生成。</span
          >
        </div>
      </div>
      <div class="login-panel-foot">
        云简 Lite · 轻量服务器监控与运维管理平台
      </div>
    </section>
  </div>
</template>
