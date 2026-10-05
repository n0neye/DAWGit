// Lists the UI texts a language has no translation for (they show in
// English). Texts are found where the code passes them to t() / tr() (first
// argument) and tn() (second and third).
//
//   node scripts/i18n-check.mjs            every language, counts
//   node scripts/i18n-check.mjs zh-TW      that language, the missing texts
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const src = join(dirname(fileURLToPath(import.meta.url)), "..", "src");
const locales = join(src, "lib", "locales");
// Texts from data (a preset's tool name), passed to t() as variables.
const extra = ["your apps"];

function files(dir) {
  return readdirSync(dir).flatMap((n) => {
    const p = join(dir, n);
    if (statSync(p).isDirectory()) return n === "locales" ? [] : files(p);
    return /\.(svelte|ts)$/.test(n) ? [p] : [];
  });
}

// The arguments of the call whose "(" is at s[i], as source text.
function args(s, i) {
  let depth = 0, start = i + 1;
  const out = [];
  for (let j = i; j < s.length; j++) {
    const c = s[j];
    if (c === '"' || c === "'" || c === "`") {
      for (j++; j < s.length && s[j] !== c; j++) if (s[j] === "\\") j++;
      continue;
    }
    if ("([{".includes(c)) depth++;
    else if (")]}".includes(c)) {
      if (--depth === 0) { out.push(s.slice(start, j)); return out; }
    } else if (c === "," && depth === 1) { out.push(s.slice(start, j)); start = j + 1; }
  }
  return out;
}

const keys = new Set(extra);
for (const f of files(src)) {
  const s = readFileSync(f, "utf8");
  for (const m of s.matchAll(/(?<![\w.$])(t|tr|tn)\(/g)) {
    const a = args(s, m.index + m[0].length - 1);
    for (const text of m[1] === "tn" ? a.slice(1, 3) : a.slice(0, 1)) {
      for (const lit of text.matchAll(/"((?:[^"\\]|\\.)*)"/g)) {
        const k = JSON.parse(`"${lit[1]}"`);
        if (k) keys.add(k);
      }
    }
  }
}

const only = process.argv[2];
for (const f of readdirSync(locales).filter((n) => n.endsWith(".json"))) {
  const lang = f.slice(0, -5);
  if (only && lang !== only) continue;
  const dict = JSON.parse(readFileSync(join(locales, f), "utf8"));
  const missing = [...keys].filter((k) => !dict[k]);
  console.log(`${lang}: ${keys.size - missing.length}/${keys.size}${missing.length ? `, ${missing.length} missing` : ""}`);
  if (only) for (const k of missing) console.log("  " + k);
  if (missing.length) process.exitCode = 1; // CI fails on untranslated text
}
