import { reactive, ref, watch } from "vue";
import {
  APPEARANCE_KEY,
  defaults,
  parseAppearance,
  normalizeAppearance,
  normalizeDashboard,
  dashboardKey,
  applyAppearance,
} from "./appearance-core.js";
export const storageAvailable = ref(true);
function read(key) {
  try {
    return localStorage.getItem(key);
  } catch {
    storageAvailable.value = false;
    return null;
  }
}
function write(key, value) {
  try {
    localStorage.setItem(key, JSON.stringify(value));
  } catch {
    storageAvailable.value = false;
  }
}
export const appearance = reactive(parseAppearance(read(APPEARANCE_KEY)));
export const dashboard = reactive(normalizeDashboard(null));
let account = null;
const dark = matchMedia("(prefers-color-scheme: dark)"),
  reduced = matchMedia("(prefers-reduced-motion: reduce)");
const apply = () =>
  applyAppearance(appearance, document, dark.matches, reduced.matches);
apply();
dark.addEventListener("change", apply);
reduced.addEventListener("change", apply);
watch(appearance, apply, { flush: "sync" });
export function updateAppearance(patch) {
  Object.assign(appearance, normalizeAppearance({ ...appearance, ...patch }));
  write(APPEARANCE_KEY, appearance);
}
export function resetAppearance() {
  updateAppearance(defaults);
}
export function loadDashboard(id) {
  account = id ?? null;
  let value = null;
  try {
    value = JSON.parse(account === null ? null : read(dashboardKey(account)));
  } catch {
    /* Use defaults. */
  }
  Object.assign(dashboard, normalizeDashboard(value));
}
export function updateDashboard(patch) {
  Object.assign(dashboard, normalizeDashboard({ ...dashboard, ...patch }));
  if (account !== null) write(dashboardKey(account), dashboard);
}
export function resetDashboard() {
  updateDashboard(normalizeDashboard(null));
}
window.addEventListener("storage", (event) => {
  if (event.key === APPEARANCE_KEY || event.key === null)
    Object.assign(appearance, parseAppearance(event.newValue));
  if (
    account !== null &&
    (event.key === dashboardKey(account) || event.key === null)
  ) {
    let value;
    try {
      value = JSON.parse(event.newValue);
    } catch {
      value = null;
    }
    Object.assign(dashboard, normalizeDashboard(value));
  }
});
