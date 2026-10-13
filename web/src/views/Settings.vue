<script setup>
import { ref, computed, onMounted } from "vue";
import { ElMessageBox, ElMessage } from "element-plus";
import { request } from "../api";
import { user } from "../state";
const admin = computed(() => user.value?.role === "admin"),
  users = ref([]),
  error = ref(""),
  busy = ref(false),
  open = ref(false),
  form = ref({ username: "", password: "", role: "viewer" });
const roles = {
  admin: "管理员 · 全部权限",
  operator: "运维员 · 服务器管理",
  viewer: "只读用户 · 数据查看",
};
async function refresh() {
  if (!admin.value) return;
  try {
    users.value = await request("/users");
  } catch (e) {
    error.value = e.message;
  }
}
async function create() {
  busy.value = true;
  error.value = "";
  try {
    await request("/users", {
      method: "POST",
      body: JSON.stringify(form.value),
    });
    open.value = false;
    form.value = { username: "", password: "", role: "viewer" };
    ElMessage.success("用户已创建");
    await refresh();
  } catch (e) {
    error.value = e.message;
  } finally {
    busy.value = false;
  }
}
async function save(row) {
  try {
    await ElMessageBox.confirm(
      `更新 ${row.username} 的角色？该用户现有登录会话将失效。`,
      "更新权限",
      { confirmButtonText: "更新", cancelButtonText: "取消" },
    );
    busy.value = true;
    await request("/users/" + row.id, {
      method: "PATCH",
      body: JSON.stringify({ role: row.role }),
    });
    ElMessage.success("权限已更新");
    if (row.id === user.value.id)
      window.dispatchEvent(new Event("session-expired"));
    else await refresh();
  } catch (e) {
    if (e instanceof Error) error.value = e.message;
  } finally {
    busy.value = false;
  }
}
async function reset(row) {
  try {
    const { value } = await ElMessageBox.prompt(
      "输入 12–72 字节的新密码，旧登录会话将失效。",
      "重置用户密码",
      {
        inputType: "password",
        confirmButtonText: "重置",
        cancelButtonText: "取消",
        inputValidator: (v) =>
          (v &&
            new TextEncoder().encode(v).length >= 12 &&
            new TextEncoder().encode(v).length <= 72) ||
          "密码须为 12–72 字节",
      },
    );
    busy.value = true;
    await request("/users/" + row.id, {
      method: "PATCH",
      body: JSON.stringify({ role: row.role, password: value }),
    });
    ElMessage.success("密码已重置");
    if (row.id === user.value.id)
      window.dispatchEvent(new Event("session-expired"));
  } catch (e) {
    if (e instanceof Error) error.value = e.message;
  } finally {
    busy.value = false;
  }
}
async function remove(row) {
  try {
    await ElMessageBox.confirm(
      `删除用户 ${row.username}？现有登录会话将失效。`,
      "删除用户",
      { confirmButtonText: "删除", cancelButtonText: "取消" },
    );
    busy.value = true;
    await request("/users/" + row.id, { method: "DELETE" });
    await refresh();
  } catch (e) {
    if (e instanceof Error) error.value = e.message;
  } finally {
    busy.value = false;
  }
}
onMounted(refresh);
</script>
<template>
  <div class="page-heading">
    <div>
      <span class="eyebrow">SETTINGS</span>
      <h1>设置</h1>
      <p>管理账户角色与权限。</p>
    </div>
    <el-button v-if="admin" :disabled="busy" type="primary" @click="open = true"
      >添加用户</el-button
    >
  </div>
  <el-alert
    v-if="error"
    :title="error"
    type="error"
    :closable="false"
    class="page-error"
  />
  <div class="panel management-panel">
    <h2>当前账户</h2>
    <p>{{ user?.username }} · {{ roles[user?.role] || "未知角色" }}</p>
    <p class="muted small">
      管理员拥有全部权限，运维员可管理服务器和执行白名单操作，只读用户可查看监控与运维数据。
    </p>
  </div>
  <div v-if="admin" class="panel management-panel">
    <h2>用户与角色</h2>
    <el-table :data="users"
      ><el-table-column
        prop="username"
        label="账户"
        min-width="150"
      /><el-table-column label="角色" min-width="250"
        ><template #default="{ row }"
          ><el-select v-model="row.role" :disabled="busy"
            ><el-option
              v-for="(label, value) in roles"
              :key="value"
              :value="value"
              :label="label" /></el-select></template></el-table-column
      ><el-table-column label="操作" min-width="260"
        ><template #default="{ row }"
          ><el-button link :disabled="busy" @click="save(row)"
            >保存角色</el-button
          ><el-button link :disabled="busy" @click="reset(row)"
            >重置密码</el-button
          ><el-button
            link
            :disabled="busy || row.id === user?.id"
            @click="remove(row)"
            >删除</el-button
          ></template
        ></el-table-column
      ></el-table
    >
  </div>
  <el-dialog
    v-model="open"
    title="添加用户"
    width="min(460px,92vw)"
    @closed="form = { username: '', password: '', role: 'viewer' }"
    ><el-form label-position="top" @submit.prevent="create"
      ><el-form-item label="账号"
        ><el-input
          v-model="form.username"
          maxlength="64"
          autocomplete="off" /></el-form-item
      ><el-form-item label="密码 · 12–72 字节"
        ><el-input
          v-model="form.password"
          type="password"
          show-password
          autocomplete="new-password" /></el-form-item
      ><el-form-item label="角色"
        ><el-select v-model="form.role"
          ><el-option
            v-for="(label, value) in roles"
            :key="value"
            :value="value"
            :label="label" /></el-select
      ></el-form-item>
      <p v-if="error" class="form-error">{{ error }}</p>
      <el-button native-type="submit" type="primary" :loading="busy"
        >创建用户</el-button
      ></el-form
    ></el-dialog
  >
</template>
