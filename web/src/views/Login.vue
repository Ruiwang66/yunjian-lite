<script setup>
import { ref } from "vue";
import { useRouter } from "vue-router";
import {
  Cloud,
  ArrowRight,
  Activity,
  LockKeyhole,
  Server,
  UserRound,
  ArrowUpRight,
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
    <header class="login-page-head">
      <router-link to="/" class="login-brand"
        ><Cloud :size="26" /><span
          >云简 <span class="login-brand-lite">Lite</span></span
        ></router-link
      >
      <router-link to="/" class="login-back"
        >返回前台监控<ArrowUpRight :size="16"
      /></router-link>
    </header>
    <div class="login-card">
      <section class="login-story">
        <div class="login-intro">
          <div class="login-cloud-mark" aria-hidden="true">
            <Cloud :size="38" />
          </div>
          <span class="eyebrow light">YOUR INFRASTRUCTURE. SIMPLIFIED.</span>
          <h1>每一台服务器，<br />尽在掌握。</h1>
          <p>
            让复杂的基础设施，回归简单。<br />在一个工作空间里，关注每一次变化。
          </p>
          <div class="login-features">
            <div>
              <Activity :size="19" /><span
                ><strong>实时监控</strong
                ><small>关键指标，每 4 秒更新</small></span
              >
            </div>
            <div>
              <Server :size="19" /><span
                ><strong>集中管理</strong
                ><small>服务器与运维操作，一处掌控</small></span
              >
            </div>
            <div>
              <LockKeyhole :size="19" /><span
                ><strong>私有部署</strong
                ><small>你的服务器，你的工作空间</small></span
              >
            </div>
          </div>
        </div>
        <div class="login-story-foot">
          CLOUDEASE LITE <span>v0.2.0 / OPEN SOURCE</span>
        </div>
      </section>
      <section class="login-panel">
        <div class="login-form">
          <span class="eyebrow">WELCOME BACK</span>
          <h2>欢迎回来</h2>
          <p class="muted">登录云简，继续管理你的服务器。</p>
          <el-form label-position="top" @submit.prevent="submit"
            ><el-form-item label="账号"
              ><el-input
                v-model="form.username"
                autocomplete="username"
                size="large"
                placeholder="admin"
                maxlength="64"
                ><template #prefix
                  ><UserRound :size="17" /></template></el-input></el-form-item
            ><el-form-item label="密码"
              ><el-input
                v-model="form.password"
                autocomplete="current-password"
                size="large"
                placeholder="输入账户密码"
                type="password"
                show-password
                maxlength="72"
                ><template #prefix
                  ><LockKeyhole
                    :size="17" /></template></el-input></el-form-item
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
              >首次管理员密码由服务端生成；其他账户请联系管理员。</span
            >
          </div>
        </div>
      </section>
    </div>
    <footer class="login-panel-foot">
      云简 Lite · 轻量服务器监控与运维管理平台
    </footer>
  </div>
</template>
