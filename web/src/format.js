export const percent = (value) =>
  Number.isFinite(value) ? value.toFixed(1) : "—";
export function bytes(value = 0, rate = false) {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let index = 0;
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024;
    index++;
  }
  return `${value.toFixed(index ? 1 : 0)} ${units[index]}${rate ? "/s" : ""}`;
}
export function ago(value, now = Date.now()) {
  if (!value) return "等待首次连接";
  const seconds = Math.max(
    0,
    Math.floor((now - new Date(value).getTime()) / 1000),
  );
  if (seconds < 60) return `${seconds} 秒前`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)} 分钟前`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} 小时前`;
  return `${Math.floor(seconds / 86400)} 天前`;
}
export function date(value) {
  return value
    ? new Date(value).toLocaleString("zh-CN", { hour12: false })
    : "—";
}
export function uptime(seconds = 0) {
  return `${Math.floor(seconds / 86400)} 天 ${Math.floor((seconds % 86400) / 3600)} 小时 ${Math.floor((seconds % 3600) / 60)} 分钟`;
}
export const statusText = {
  online: "在线",
  offline: "离线",
  pending: "待连接",
};

export function eventMessage(value = "") {
  return value.replaceAll("Agent disconnected: no report for 90 seconds", "监控客户端离线：90 秒未收到数据").replaceAll("Agent connected", "监控客户端已连接").replaceAll("Agent reconnected", "监控客户端已重新连接");
}
