import { createRouter, createWebHistory } from "vue-router";
import { api } from "./api";
import { user } from "./state";
import Login from "./views/Login.vue";
import Dashboard from "./views/Dashboard.vue";
import Server from "./views/Server.vue";
import Logs from "./views/Logs.vue";
import Public from "./views/Public.vue";
const Docker = () => import("./views/Docker.vue");
const Services = () => import("./views/Services.vue");
const Files = () => import("./views/Files.vue");
const Security = () => import("./views/Security.vue");
const System = () => import("./views/System.vue");
const Operations = () => import("./views/Operations.vue");
const Settings = () => import("./views/Settings.vue");
const Website = () => import("./views/Website.vue");
const SSL = () => import("./views/SSL.vue");
const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/login", component: Login, meta: { public: true } },
    { path: "/", component: Public, meta: { public: true } },
    { path: "/status/servers/:id", component: Public, meta: { public: true } },
    { path: "/admin", component: Dashboard },
    { path: "/servers", component: Dashboard },
    { path: "/servers/:id", component: Server },
    { path: "/logs", component: Logs },
    { path: "/docker", component: Docker },
    { path: "/services", component: Services },
    { path: "/files", component: Files },
    { path: "/security", component: Security },
    { path: "/system", component: System },
    { path: "/operations", component: Operations },
    { path: "/settings", component: Settings },
    { path: "/websites", component: Website },
    { path: "/ssl", component: SSL },
    { path: "/:pathMatch(.*)*", redirect: "/" },
  ],
});
router.beforeEach(async (to) => {
  if (to.meta.public) return true;
  if (!user.value) {
    try {
      user.value = await api.me();
    } catch {
      return "/login";
    }
  }
  return true;
});
export default router;
