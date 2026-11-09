export type ViewName = "galaxy" | "atlas";
export type Mode = "light" | "dark";

// Per-device conveniences: losing them (private window, cleared storage) only resets the look.
function pref<T extends string>(key: string, allowed: readonly T[], fallback: () => T) {
  return {
    get(): T {
      try {
        const v = localStorage.getItem(key);
        if (v && (allowed as readonly string[]).includes(v)) return v as T;
      } catch { /* storage blocked */ }
      return fallback();
    },
    set(v: T) {
      try { localStorage.setItem(key, v); } catch { /* storage blocked */ }
    },
  };
}

export const prefs = {
  view: pref<ViewName>("rtk.view", ["galaxy", "atlas"], () => "galaxy"),
  mode: pref<Mode>("rtk.mode", ["light", "dark"], () => (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light")),
};

export function applyTheme(view: ViewName, mode: Mode) {
  const theme = view === "atlas" ? `atlas-${mode}` : "space";
  document.documentElement.dataset.theme = theme;
  document.querySelector('meta[name="color-scheme"]')?.setAttribute("content", theme === "atlas-light" ? "light" : "dark");
}
