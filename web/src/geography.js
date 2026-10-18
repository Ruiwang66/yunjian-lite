import data from "./components/countries.json";
const common = [
  "CN",
  "HK",
  "MO",
  "TW",
  "SG",
  "JP",
  "KR",
  "US",
  "CA",
  "GB",
  "DE",
  "FR",
  "NL",
  "AU",
];
export const countries = [...data].sort((a, b) => {
  const ai = common.indexOf(a.code),
    bi = common.indexOf(b.code);
  if (ai >= 0 || bi >= 0) return (ai < 0 ? 999 : ai) - (bi < 0 ? 999 : bi);
  return a.name.localeCompare(b.name, "zh-CN");
});
const names = new Map(data.map((c) => [c.code, c.name]));
export const countryName = (code) => names.get(code) || "未设置地区";
export function distribution(servers) {
  const grouped = new Map();
  let unknown = 0;
  for (const server of servers) {
    if (!names.has(server.country_code)) {
      unknown++;
      continue;
    }
    let group = grouped.get(server.country_code);
    if (!group) {
      group = {
        code: server.country_code,
        name: countryName(server.country_code),
        total: 0,
        online: 0,
        servers: [],
      };
      grouped.set(group.code, group);
    }
    group.total++;
    if (server.status === "online") group.online++;
    group.servers.push(server);
  }
  return {
    groups: [...grouped.values()].sort(
      (a, b) => b.total - a.total || a.name.localeCompare(b.name, "zh-CN"),
    ),
    unknown,
  };
}
