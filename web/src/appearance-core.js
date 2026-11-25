// Shared by the blocking first-paint script, Vue settings and Node tests.
export const APPEARANCE_KEY = "cloudease.appearance.v1";
export const dashboardKey = (id) => `cloudease.dashboard.v1.${id}`;
export const defaults = Object.freeze({
  scheme: "light",
  accent: "violet",
  publicLayout: "compact",
  density: "comfortable",
  glass: "light",
  motion: "system",
});
const choices = {
  scheme: ["light", "dark", "system"],
  accent: ["graphite", "blue", "teal", "violet"],
  publicLayout: ["compact"],
  density: ["comfortable", "compact"],
  glass: ["off", "light", "clear"],
  motion: ["system", "reduced"],
};
export function normalizeAppearance(value) {
  const input = value && typeof value === "object" ? value : {};
  return Object.fromEntries(
    Object.entries(defaults).map(([key, fallback]) => [
      key,
      choices[key].includes(input[key]) ? input[key] : fallback,
    ]),
  );
}
export function parseAppearance(raw) {
  try {
    return normalizeAppearance(JSON.parse(raw));
  } catch {
    return { ...defaults };
  }
}
export function normalizeDashboard(value) {
  const sections = [
    ...new Set(
      (Array.isArray(value?.sections) ? value.sections : []).filter((x) =>
        ["map", "activity"].includes(x),
      ),
    ),
  ];
  for (const key of ["map", "activity"])
    if (!sections.includes(key)) sections.push(key);
  return {
    sections,
    mapExpanded:
      typeof value?.mapExpanded === "boolean" ? value.mapExpanded : true,
  };
}
export function effectiveAppearance(value, dark, reduced) {
  return {
    theme: value.scheme === "system" ? (dark ? "dark" : "light") : value.scheme,
    motion: reduced || value.motion === "reduced" ? "reduced" : "standard",
  };
}
export function applyAppearance(value, doc, dark = false, reduced = false) {
  const actual = effectiveAppearance(value, dark, reduced);
  Object.assign(doc.documentElement.dataset, {
    theme: actual.theme,
    accent: value.accent,
    density: value.density,
    glass: value.glass,
    motion: actual.motion,
  });
  doc.documentElement.style.colorScheme = actual.theme;
  doc
    .querySelector('meta[name="theme-color"]')
    ?.setAttribute("content", actual.theme === "dark" ? "#101419" : "#fcfcfe");
}
