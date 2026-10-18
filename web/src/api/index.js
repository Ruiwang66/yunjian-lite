// Same-origin cookies keep session secrets out of localStorage and URLs.
export async function request(path, options = {}) {
  const response = await fetch("/api" + path, {
    credentials: "same-origin",
    ...options,
    headers: { "Content-Type": "application/json", ...options.headers },
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    if (response.status === 401 && path !== "/auth/login")
      window.dispatchEvent(new Event("session-expired"));
    throw new Error(body.error || `请求失败 (${response.status})`);
  }
  return body;
}
export const publicApi = {
  servers: () => request("/public/servers"),
  server: (id) => request("/public/servers/" + id),
  history: (id, hours) =>
    request("/public/servers/" + id + "/metrics?hours=" + hours),
};
export const api = {
  me: () => request("/auth/me"),
  login: (data) =>
    request("/auth/login", { method: "POST", body: JSON.stringify(data) }),
  logout: () => request("/auth/logout", { method: "POST" }),
  password: (data) =>
    request("/auth/password", { method: "POST", body: JSON.stringify(data) }),
  servers: () => request("/servers"),
  server: (id) => request(`/servers/${id}`),
  create: (name, country_code = "") =>
    request("/servers", {
      method: "POST",
      body: JSON.stringify({ name, country_code }),
    }),
  rename: (id, name) =>
    request(`/servers/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ name }),
    }),
  uninstall: (id) => request(`/servers/${id}/uninstall`),
  location: (id, country_code) =>
    request(`/servers/${id}/location`, {
      method: "PATCH",
      body: JSON.stringify({ country_code }),
    }),
  remove: (id) => request(`/servers/${id}`, { method: "DELETE" }),
  token: (id) => request(`/servers/${id}/token`, { method: "POST" }),
  history: (id, hours) => request(`/servers/${id}/metrics?hours=${hours}`),
  logs: (params = {}) => request("/logs?" + new URLSearchParams(params)),
};
