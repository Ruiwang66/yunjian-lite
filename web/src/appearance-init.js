import {
  APPEARANCE_KEY,
  parseAppearance,
  applyAppearance,
} from "./appearance-core.js";
let raw = null;
try {
  raw = localStorage.getItem(APPEARANCE_KEY);
} catch {
  /* Storage is optional. */
}
applyAppearance(
  parseAppearance(raw),
  document,
  matchMedia("(prefers-color-scheme: dark)").matches,
  matchMedia("(prefers-reduced-motion: reduce)").matches,
);
