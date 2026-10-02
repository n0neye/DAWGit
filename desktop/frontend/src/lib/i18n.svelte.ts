// The app's languages. Texts are written in English in the code and are
// their own keys: t("Commit & Share") is looked up in the language's file
// (locales/<code>.json, English text -> translation), and stays English
// where a translation is missing. {name} in a text is filled from vars.

export const languages = [
  { code: "en", name: "English" },
  { code: "zh-TW", name: "繁體中文" },
  { code: "zh-CN", name: "简体中文" },
  { code: "ja", name: "日本語" },
  { code: "ko", name: "한국어" },
  { code: "es", name: "Español" },
  { code: "fr", name: "Français" },
  { code: "de", name: "Deutsch" },
  { code: "pt-BR", name: "Português (Brasil)" },
  { code: "ru", name: "Русский" },
  { code: "it", name: "Italiano" },
] as const;

const KEY = "dawgit.lang";
const files = import.meta.glob<Record<string, string>>("./locales/*.json", { import: "default" });

// The language the system asks for, as one of ours.
function systemLanguage(): string {
  for (const want of navigator.languages ?? [navigator.language]) {
    const w = want.toLowerCase();
    if (w.startsWith("zh")) return /tw|hk|mo|hant/.test(w) ? "zh-TW" : "zh-CN";
    if (w.startsWith("pt")) return "pt-BR";
    const hit = languages.find((l) => l.code.toLowerCase() === w.slice(0, 2));
    if (hit) return hit.code;
  }
  return "en";
}

function saved(): string {
  try {
    return localStorage.getItem(KEY) ?? "";
  } catch {
    return "";
  }
}

const state = $state<{ code: string; dict: Record<string, string> }>({ code: "en", dict: {} });

/** The language in use (a code from languages). */
export function language(): string {
  return state.code;
}

/** "" follows the system's language. */
export function chosenLanguage(): string {
  return saved();
}

async function load(code: string) {
  const file = files[`./locales/${code}.json`];
  const dict = file ? await file().catch(() => ({})) : {};
  state.code = code;
  state.dict = dict;
  document.documentElement.lang = code;
}

/** Sets the language ("" follows the system's) and remembers it. */
export async function setLanguage(code: string) {
  try {
    if (code) localStorage.setItem(KEY, code);
    else localStorage.removeItem(KEY);
  } catch {
    /* not remembered: still used now */
  }
  await load(code || systemLanguage());
}

/** Loads the language to use; the app waits for it before showing texts. */
export function startLanguage(): Promise<void> {
  return load(saved() || systemLanguage());
}

function fill(text: string, vars?: Record<string, string | number>): string {
  if (!vars) return text;
  return text.replace(/\{(\w+)\}/g, (m, k) => (k in vars ? String(vars[k]) : m));
}

/** A text in the language in use. */
export function t(text: string, vars?: Record<string, string | number>): string {
  return fill(state.dict[text] || text, vars);
}

/** One of two texts by count: tn(n, "{n} file", "{n} files"); n is in vars. */
export function tn(n: number, one: string, other: string, vars?: Record<string, string | number>): string {
  return t(n === 1 ? one : other, { n, ...vars });
}
