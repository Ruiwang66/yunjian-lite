<script setup>
import { ref } from "vue";
import { ElMessage } from "element-plus";
import TokenSetup from "./TokenSetup.vue";
import { api } from "../api";
const open = defineModel({ type: Boolean }),
  emit = defineEmits(["created"]);
const name = ref(""),
  token = ref(""),
  loading = ref(false);
async function submit() {
  if (!name.value.trim()) return ElMessage.info("请输入服务器名称");
  loading.value = true;
  try {
    const result = await api.create(name.value.trim());
    token.value = result.token;
    emit("created");
    ElMessage.success("服务器已创建");
  } catch (e) {
    ElMessage.error(e.message);
  } finally {
    loading.value = false;
  }
}
function reset() {
  name.value = "";
  token.value = "";
}
</script>
<template>
  <el-dialog
    v-model="open"
    :title="token ? '连接你的服务器' : '添加服务器'"
    width="min(640px, 92vw)"
    @closed="reset"
    ><TokenSetup v-if="token" :token="token" /><el-form
      v-else
      label-position="top"
      @submit.prevent="submit"
      ><p class="muted">创建一枚专属连接密钥，连接你的第一台服务器。</p>
      <el-form-item label="服务器名称"
        ><el-input
          v-model="name"
          placeholder="例如：Debian 生产服务器"
          maxlength="80"
          autofocus /></el-form-item
      ><el-button type="primary" :loading="loading" native-type="submit"
        >创建并获取连接密钥</el-button
      ></el-form
    ><template #footer
      ><el-button @click="open = false">{{
        token ? "完成" : "取消"
      }}</el-button></template
    ></el-dialog
  >
</template>
