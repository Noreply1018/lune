import fs from "node:fs";
import path from "node:path";
import ts from "typescript";

const root = path.resolve(import.meta.dirname, "..");
const sourcePath = path.join(root, "src/lib/codexQuota.ts");
let source = fs.readFileSync(sourcePath, "utf8");
source = source
  .replace(/import type \{ Account \} from "@\/lib\/types";\n/, "")
  .replace(/export type /g, "type ")
  .replace(/export interface /g, "interface ")
  .replace(/export function /g, "function ");
source += "\nmodule.exports = { parseCodexQuota, codexWeeklyWindowMeta };\n";

const js = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    target: ts.ScriptTarget.ES2020,
  },
}).outputText;
const mod = { exports: {} };
new Function("module", "exports", js)(mod, mod.exports);

const { parseCodexQuota, codexWeeklyWindowMeta } = mod.exports;

function account(provider, quota) {
  return {
    cpa_provider: provider,
    codex_quota_json: quota == null ? "" : JSON.stringify(quota),
  };
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const valid = account("CoDeX", {
  rate_limit: {
    primary_window: {
      used_percent: "42.5",
      limit_window_seconds: "300",
      reset_after_seconds: "120",
      reset_at: "1800000000",
    },
    secondary_window: {
      used_percent: 7,
      limit_window_seconds: 604800,
      reset_after_seconds: 60,
      reset_at: 1800000060,
    },
    allowed: true,
    limit_reached: false,
  },
});
const parsed = parseCodexQuota(valid);
assert(parsed, "numeric-string quota did not parse");
assert(parsed.primary.usedPercent === 42.5, "primary usedPercent mismatch");
assert(parsed.primary.limitWindowSeconds === 300, "primary limit window mismatch");
assert(parsed.secondary?.usedPercent === 7, "secondary usedPercent mismatch");

const invalid = account("codex", {
  rate_limit: {
    primary_window: {
      used_percent: "abc",
      limit_window_seconds: 300,
      reset_after_seconds: 120,
      reset_at: 1800000000,
    },
  },
});
assert(parseCodexQuota(invalid) === null, "invalid used_percent must be rejected");

const missingUsed = account("codex", {
  rate_limit: {
    primary_window: {
      limit_window_seconds: 300,
      reset_after_seconds: 120,
      reset_at: 1800000000,
    },
  },
});
const missingParsed = parseCodexQuota(missingUsed);
assert(missingParsed?.primary.usedPercent === 0, "missing used_percent should default to zero");

const labels = {
  fetch401: codexWeeklyWindowMeta("plus", "额度查询失败"),
  plusPrimaryOnly: codexWeeklyWindowMeta("plus"),
  freePrimaryOnly: codexWeeklyWindowMeta("free"),
  unknownPrimaryOnly: codexWeeklyWindowMeta("unknown"),
};
assert(labels.fetch401.text === "额度查询失败", "fetch401 label mismatch");
assert(labels.plusPrimaryOnly.text === "周额度待同步", "plus primary-only label mismatch");
assert(labels.freePrimaryOnly.text === "无周额度", "free primary-only label mismatch");
assert(labels.unknownPrimaryOnly.text === "周额度未知", "unknown primary-only label mismatch");

console.log(
  JSON.stringify(
    {
      status: "ok",
      parser: "case-insensitive provider, numeric strings, invalid rejection, missing used_percent default",
      labels,
    },
    null,
    2,
  ),
);
