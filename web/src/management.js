import { ref, reactive, computed, watch, onMounted, onUnmounted } from "vue";
import { useRoute } from "vue-router";
import { api, request } from "./api";
import { user } from "./state";

let selectedServer = null;
export const canOperate = computed(() =>
  ["admin", "operator"].includes(user.value?.role),
);
export async function operation(path, params, method = "GET", signal) {
  let result = await request(
    path + (method === "GET" ? "?" + new URLSearchParams(params) : ""),
    {
      method,
      signal,
      ...(method === "POST" ? { body: JSON.stringify(params) } : {}),
    },
  );
  if (!result.id || result.status !== "queued") return result;
  const deadline = Date.now() + 50000;
  while (Date.now() < deadline) {
    if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
    await new Promise((resolve) => setTimeout(resolve, 800));
    result = await request("/operations/" + result.id, { signal });
    if (result.status === "success") {
      if (result.data == null) throw new Error("结果已清理，请重新读取");
      return result.data;
    }
    if (["failed", "expired", "cancelled"].includes(result.status)) {
      throw new Error(
        result.error ||
          (result.status === "expired"
            ? "操作已过期，执行状态可能未知，请刷新确认后再操作"
            : "操作已取消"),
      );
    }
  }
  throw new Error("等待操作结果超时，请在运维日志中检查状态");
}
export function useManagement() {
  const route = useRoute();
  const servers = ref([]),
    selected = ref(Number(route.query.server_id) || selectedServer),
    info = ref(null);
  const loading = ref(true),
    busy = ref(false),
    error = ref("");
  let generation = 0,
    controller;
  async function loadInfo() {
    const current = ++generation;
    controller?.abort();
    controller = new AbortController();
    info.value = null;
    busy.value = false;
    error.value = "";
    selectedServer = selected.value;
    if (!selected.value) {
      loading.value = false;
      return;
    }
    loading.value = true;
    try {
      const result = await request(
        "/management/info?server_id=" + selected.value,
        { signal: controller.signal },
      );
      if (current === generation) info.value = result;
    } catch (e) {
      if (e.name !== "AbortError" && current === generation)
        error.value = e.message;
    } finally {
      if (current === generation) loading.value = false;
    }
  }
  async function execute(path, params = {}, method = "GET") {
    if (!info.value || busy.value)
      throw new Error("请先选择服务器并等待当前操作完成");
    const current = generation;
    busy.value = true;
    error.value = "";
    try {
      const result = await operation(
        path,
        { ...params, server_id: selected.value },
        method,
        controller.signal,
      );
      if (current !== generation)
        throw new DOMException("Aborted", "AbortError");
      return result;
    } catch (e) {
      if (e.name !== "AbortError" && current === generation)
        error.value = e.message;
      throw e;
    } finally {
      if (current === generation) busy.value = false;
    }
  }
  watch(selected, loadInfo);
  onMounted(async () => {
    try {
      servers.value = await api.servers();
      if (!servers.value.some((s) => s.id === selected.value))
        selected.value = servers.value[0]?.id || null;
      else await loadInfo();
      if (!selected.value) loading.value = false;
    } catch (e) {
      error.value = e.message;
      loading.value = false;
    }
  });
  onUnmounted(() => {
    generation++;
    controller?.abort();
  });
  return reactive({
    servers,
    selected,
    info,
    loading,
    busy,
    error,
    execute,
    loadInfo,
    canOperate,
  });
}
export function decodeFile(content) {
  return Uint8Array.from(atob(content), (c) => c.charCodeAt(0));
}
