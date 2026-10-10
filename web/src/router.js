import { createRouter, createWebHistory } from "vue-router";
import { api } from "./api";
import { user } from "./state";
import Login from "./views/Login.vue";
import Dashboard from "./views/Dashboard.vue";
import Server from "./views/Server.vue";
import Logs from "./views/Logs.vue";
import Public from "./views/Public.vue";
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
