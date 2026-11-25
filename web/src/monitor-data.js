// Pure calculations: missing values never silently become zero.
export const numeric = (value) =>
  typeof value === "number" && Number.isFinite(value) && value >= 0;
export function sumField(servers, key, onlineOnly = false) {
  const eligible = onlineOnly
    ? servers.filter((s) => s.status === "online")
    : servers;
  const values = eligible.map((s) => s.latest?.[key]).filter(numeric);
  return {
    value: values.length ? values.reduce((a, b) => a + b, 0) : null,
    covered: values.length,
    total: eligible.length,
  };
}
export function sampleSignature(servers) {
  return servers
    .map(
      (s) =>
        `${s.id}:${s.status}:${s.latest?.id ?? ""}:${s.latest?.created_at ?? ""}:${s.latest?.network_tx_rate ?? ""}:${s.latest?.network_rx_rate ?? ""}`,
    )
    .sort()
    .join("|");
}
export function appendRatePoint(points, signature, servers, now) {
  const last = points.at(-1);
  if (last?.signature === signature) return points;
  const tx = sumField(servers, "network_tx_rate", true).value,
    rx = sumField(servers, "network_rx_rate", true).value;
  const kept = points.filter((p) => now - p.time <= 60000);
  return [...kept, { time: now, tx, rx, signature }];
}
export function ratePath(points, key, now) {
  const recent = points.filter((p) => now - p.time <= 60000);
  const max = Math.max(
    1,
    ...recent.flatMap((p) => [p.tx, p.rx]).filter(numeric),
  );
  let previous = null;
  return recent
    .map((p) => {
      if (!numeric(p[key])) {
        previous = null;
        return "";
      }
      const command =
        previous !== null && p.time - previous <= 12000 ? "L" : "M";
      previous = p.time;
      return `${command}${(((p.time - (now - 60000)) / 60000) * 300).toFixed(1)},${(46 - (p[key] / max) * 40).toFixed(1)}`;
    })
    .join(" ");
}
