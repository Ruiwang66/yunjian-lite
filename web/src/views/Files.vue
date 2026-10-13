<script setup>
import { ref, watch, computed } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import ManagementHeader from "../components/ManagementHeader.vue";
import { useManagement, decodeFile } from "../management";
import { bytes } from "../format";
const scope = useManagement(),
  root = ref(0),
  path = ref("."),
  entries = ref([]),
  preview = ref(""),
  previewOpen = ref(false),
  input = ref(null);
const enabled = computed(
  () =>
    scope.info?.capabilities.enabled && scope.info?.server.status === "online",
);
async function refresh() {
  entries.value = [];
  if (!enabled.value) return;
  try {
    entries.value = await scope.execute("/files/list", {
      root: root.value,
      path: path.value,
    });
  } catch {}
}
watch(
  () => scope.info,
  () => {
    root.value = 0;
    path.value = ".";
    refresh();
  },
);
async function chooseRoot() {
  path.value = ".";
  await refresh();
}
function target(name) {
  return path.value === "." ? name : path.value + "/" + name;
}
async function open(row) {
  if (row.directory) {
    path.value = target(row.name);
    await refresh();
    return;
  }
  try {
    const data = await scope.execute("/files/read", {
      root: root.value,
      path: target(row.name),
    });
    preview.value = new TextDecoder("utf-8", { fatal: true }).decode(
      decodeFile(data.content),
    );
    previewOpen.value = true;
  } catch (e) {
    if (!scope.error) scope.error = e.message;
  }
}
async function download(row) {
  try {
    const data = await scope.execute("/files/download", {
      root: root.value,
      path: target(row.name),
    });
    const url = URL.createObjectURL(
      new Blob([decodeFile(data.content)], {
        type: "application/octet-stream",
      }),
    );
    const link = document.createElement("a");
    link.href = url;
    link.download = row.name;
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  } catch {}
}
async function upload(event) {
  const file = event.target.files?.[0];
  event.target.value = "";
  if (!file) return;
  if (file.size > 32768) {
    ElMessage.error("文件最大 32 KiB");
    return;
  }
  try {
    await ElMessageBox.confirm(
      `上传 ${file.name} 到当前目录？同名文件不会被覆盖。`,
      "上传文件",
      { confirmButtonText: "上传", cancelButtonText: "取消" },
    );
    const data = new Uint8Array(await file.arrayBuffer());
    new TextDecoder("utf-8", { fatal: true }).decode(data);
    const content = btoa(
      Array.from(data, (b) => String.fromCharCode(b)).join(""),
    );
    await scope.execute(
      "/files/upload",
      { root: root.value, path: target(file.name), content },
      "POST",
    );
    ElMessage.success("上传完成");
    await refresh();
  } catch (e) {
    if (e instanceof Error && !scope.error) scope.error = e.message;
  }
}
function back() {
  path.value = path.value.includes("/")
    ? path.value.slice(0, path.value.lastIndexOf("/"))
    : ".";
  refresh();
}
</script>
<template>
  <ManagementHeader
    title="文件管理"
    description="浏览客户端明确允许的目录，查看、下载与上传小型文本文件。"
    :scope="scope"
  />
  <div v-if="scope.info" class="panel management-panel">
    <div class="section-heading">
      <h2>允许目录</h2>
      <el-button
        :disabled="
          scope.busy ||
          !enabled ||
          !scope.canOperate ||
          !scope.info.capabilities.file_write
        "
        @click="input.click()"
        >上传文本文件</el-button
      >
    </div>
    <div class="file-path-row">
      <el-select
        v-model="root"
        :disabled="scope.busy"
        placeholder="没有允许目录"
        @change="chooseRoot"
        ><el-option
          v-for="(dir, index) in scope.info.capabilities.file_roots || []"
          :key="dir"
          :label="dir"
          :value="index" /></el-select
      ><code>{{ path }}</code
      ><el-button :disabled="scope.busy || path === '.'" @click="back"
        >上一级</el-button
      ><el-button :disabled="scope.busy || !enabled" @click="refresh"
        >刷新</el-button
      >
    </div>
    <input
      ref="input"
      hidden
      type="file"
      accept=".txt,.md,.csv,.json,.yaml,.yml,.log"
      @change="upload"
    />
    <el-table
      :data="entries"
      empty-text="目录为空，或允许目录不存在 / 权限不足"
    >
      <el-table-column label="名称" min-width="230"
        ><template #default="{ row }"
          ><el-button link :disabled="scope.busy" @click="open(row)"
            >{{ row.directory ? "▣" : "▤" }} {{ row.name }}</el-button
          ></template
        ></el-table-column
      >
      <el-table-column label="大小" width="130"
        ><template #default="{ row }">{{
          row.directory ? "—" : bytes(row.size)
        }}</template></el-table-column
      >
      <el-table-column label="修改时间" min-width="180"
        ><template #default="{ row }">{{
          new Date(row.modified_at * 1000).toLocaleString()
        }}</template></el-table-column
      >
      <el-table-column label="操作" width="150"
        ><template #default="{ row }"
          ><el-button
            v-if="!row.directory"
            link
            :disabled="scope.busy"
            @click="download(row)"
            >下载</el-button
          ></template
        ></el-table-column
      >
    </el-table>
    <p class="small muted">
      文件最大 32 KiB；上传仅支持 UTF-8
      文本且不覆盖文件。密钥、数据库、隐藏文件和符号链接不会列出。
    </p>
  </div>
  <el-dialog
    v-model="previewOpen"
    title="查看文件"
    width="min(900px, 94vw)"
    @closed="preview = ''"
  >
    <pre class="operation-output">{{ preview }}</pre>
  </el-dialog>
</template>
