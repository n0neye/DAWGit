// Syntax colors for code (highlight.js, loaded when first needed), as HTML
// one line at a time: line numbers and diffs show files line by line.

import type { HLJSApi, LanguageFn } from "highlight.js";

// By file extension (lowercase, with the dot) or name: the language.
const byExt: Record<string, string> = {
  ".js": "javascript", ".mjs": "javascript", ".cjs": "javascript", ".jsx": "javascript",
  ".ts": "typescript", ".tsx": "typescript", ".svelte": "xml", ".vue": "xml",
  ".py": "python", ".go": "go", ".rs": "rust", ".cs": "csharp", ".java": "java", ".kt": "kotlin", ".kts": "kotlin",
  ".swift": "swift", ".php": "php", ".rb": "ruby", ".lua": "lua", ".dart": "dart", ".sql": "sql",
  ".c": "cpp", ".h": "cpp", ".cc": "cpp", ".cpp": "cpp", ".hpp": "cpp", ".inl": "cpp",
  ".shader": "cpp", ".hlsl": "cpp", ".cginc": "cpp", ".compute": "cpp", ".usf": "cpp", ".ush": "cpp",
  ".glsl": "glsl", ".vert": "glsl", ".frag": "glsl",
  ".sh": "bash", ".bash": "bash", ".zsh": "bash", ".ps1": "powershell", ".psm1": "powershell", ".bat": "dos", ".cmd": "dos",
  ".html": "xml", ".htm": "xml", ".xml": "xml", ".uxml": "xml", ".svg": "xml", ".csproj": "xml", ".plist": "xml",
  ".css": "css", ".uss": "css", ".scss": "scss", ".sass": "scss", ".less": "less",
  ".json": "json", ".jsonc": "json", ".asmdef": "json", ".asmref": "json", ".uproject": "json", ".uplugin": "json",
  ".yaml": "yaml", ".yml": "yaml", ".toml": "ini", ".ini": "ini", ".cfg": "ini", ".conf": "ini", ".env": "ini",
  ".md": "markdown", ".markdown": "markdown", ".diff": "diff", ".patch": "diff",
  ".gradle": "gradle", ".cmake": "cmake", ".mk": "makefile", ".graphql": "graphql", ".proto": "protobuf",
};
const byName: Record<string, string> = { dockerfile: "dockerfile", makefile: "makefile", "cmakelists.txt": "cmake",
  ".gitignore": "bash", ".dawgit.yaml": "yaml" };

// The languages, each its own chunk.
const loaders: Record<string, () => Promise<{ default: LanguageFn }>> = {
  javascript: () => import("highlight.js/lib/languages/javascript"),
  typescript: () => import("highlight.js/lib/languages/typescript"),
  xml: () => import("highlight.js/lib/languages/xml"),
  python: () => import("highlight.js/lib/languages/python"),
  go: () => import("highlight.js/lib/languages/go"),
  rust: () => import("highlight.js/lib/languages/rust"),
  csharp: () => import("highlight.js/lib/languages/csharp"),
  java: () => import("highlight.js/lib/languages/java"),
  kotlin: () => import("highlight.js/lib/languages/kotlin"),
  swift: () => import("highlight.js/lib/languages/swift"),
  php: () => import("highlight.js/lib/languages/php"),
  ruby: () => import("highlight.js/lib/languages/ruby"),
  lua: () => import("highlight.js/lib/languages/lua"),
  dart: () => import("highlight.js/lib/languages/dart"),
  sql: () => import("highlight.js/lib/languages/sql"),
  cpp: () => import("highlight.js/lib/languages/cpp"),
  glsl: () => import("highlight.js/lib/languages/glsl"),
  bash: () => import("highlight.js/lib/languages/bash"),
  powershell: () => import("highlight.js/lib/languages/powershell"),
  dos: () => import("highlight.js/lib/languages/dos"),
  css: () => import("highlight.js/lib/languages/css"),
  scss: () => import("highlight.js/lib/languages/scss"),
  less: () => import("highlight.js/lib/languages/less"),
  json: () => import("highlight.js/lib/languages/json"),
  yaml: () => import("highlight.js/lib/languages/yaml"),
  ini: () => import("highlight.js/lib/languages/ini"),
  markdown: () => import("highlight.js/lib/languages/markdown"),
  diff: () => import("highlight.js/lib/languages/diff"),
  gradle: () => import("highlight.js/lib/languages/gradle"),
  cmake: () => import("highlight.js/lib/languages/cmake"),
  makefile: () => import("highlight.js/lib/languages/makefile"),
  graphql: () => import("highlight.js/lib/languages/graphql"),
  protobuf: () => import("highlight.js/lib/languages/protobuf"),
  dockerfile: () => import("highlight.js/lib/languages/dockerfile"),
};

/** The language of a file, by its name ("" when none is known). */
export function languageOf(path: string): string {
  const name = path.slice(path.lastIndexOf("/") + 1).toLowerCase();
  if (byName[name]) return byName[name];
  const dot = name.lastIndexOf(".");
  return dot >= 0 ? byExt[name.slice(dot)] ?? "" : "";
}

let core: Promise<HLJSApi> | null = null;
const loaded = new Set<string>();

async function api(language: string): Promise<HLJSApi | null> {
  if (!loaders[language]) return null;
  core ??= import("highlight.js/lib/core").then((m) => m.default);
  const hljs = await core;
  if (!loaded.has(language)) {
    hljs.registerLanguage(language, (await loaders[language]()).default);
    loaded.add(language);
  }
  return hljs;
}

// Above these, plain text (coloring would take a moment).
const maxLines = 20000, maxChars = 2_000_000;

/**
 * Lines as HTML with syntax colors, the whole text colored at once (a
 * comment over several lines stays a comment), or null when the language
 * isn't known or the text is too long.
 */
export async function highlightLines(lines: string[], language: string): Promise<string[] | null> {
  if (!language || lines.length > maxLines) return null;
  const text = lines.join("\n");
  if (text.length > maxChars) return null;
  const hljs = await api(language);
  if (!hljs) return null;
  return splitHTML(hljs.highlight(text, { language, ignoreIllegals: true }).value);
}

// splitHTML cuts highlighted HTML at line breaks, closing the spans open at
// the end of a line and opening them again on the next.
function splitHTML(html: string): string[] {
  const out: string[] = [];
  const open: string[] = [];
  let line = "";
  const re = /(<span[^>]*>)|(<\/span>)|(\n)|([^<\n]+|<)/g;
  for (let m; (m = re.exec(html)); ) {
    if (m[1]) { open.push(m[1]); line += m[1]; }
    else if (m[2]) { open.pop(); line += m[2]; }
    else if (m[3]) {
      out.push(line + "</span>".repeat(open.length));
      line = open.join("");
    } else line += m[4];
  }
  out.push(line);
  return out;
}
