// Runs the whole browser end-to-end suite: seeds a fixture pricing snapshot into a throwaway
// store, starts a REAL assessd and the Vite dev server on throwaway state, runs every spec in
// this directory, and tears everything down. Exit code is non-zero if any spec fails.
//
//   npm run e2e                      # from canvas/ — needs Go, Node and a Playwright Chromium
//   E2E_ONLY=02 npm run e2e          # just specs whose filename starts with 02
//   UI_URL=... API_URL=... npm run e2e   # use servers you already started (nothing is spawned)
import { spawn, spawnSync } from "node:child_process";
import { mkdtempSync, readdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const api = process.env.API_URL ?? "http://localhost:8099";
const ui = process.env.UI_URL ?? "http://localhost:5183";
const children = [];

async function waitFor(url, label) {
  for (let i = 0; i < 90; i++) {
    try {
      const r = await fetch(url);
      if (r.status < 500) return;
    } catch {}
    await new Promise((r) => setTimeout(r, 1000));
  }
  throw new Error(`${label} did not come up at ${url}`);
}
function stopAll() {
  for (const c of children) {
    try { process.kill(-c.pid, "SIGTERM"); } catch {}
  }
}
process.on("exit", stopAll);
process.on("SIGINT", () => { stopAll(); process.exit(130); });

const spawnServers = !process.env.UI_URL && !process.env.API_URL;
let tmp;
try {
  if (spawnServers) {
    tmp = mkdtempSync(join(tmpdir(), "preflight-e2e-"));
    const pricingDB = join(tmp, "pricing.db");
    const seed = spawnSync("go", ["run", "./tests/e2e/seedpricing", pricingDB], { cwd: root, stdio: "inherit" });
    if (seed.status !== 0) throw new Error("seeding the fixture pricing snapshot failed");
    const port = new URL(api).port || "8099";
    const uiPort = new URL(ui).port || "5183";
    const env = { ...process.env, PREFLIGHT_DB_PATH: join(tmp, "sessions.db"), PREFLIGHT_PRICING_DB_PATH: pricingDB, PREFLIGHT_ASSESSD_PORT: port };
    children.push(spawn("go", ["run", "./cmd/assessd"], { cwd: root, env, detached: true, stdio: "ignore" }));
    children.push(spawn("npx", ["vite", "--port", uiPort, "--strictPort"], { cwd: join(root, "canvas"), env: { ...process.env, VITE_ASSESSD_URL: api }, detached: true, stdio: "ignore" }));
  }
  await waitFor(`${api}/templates`, "assessd");
  await waitFor(ui, "the Vite dev server");

  const only = process.env.E2E_ONLY;
  const specs = readdirSync(here).filter((f) => /^\d\d-.*\.mjs$/.test(f) && (!only || f.startsWith(only))).sort();
  let failed = 0;
  for (const spec of specs) {
    console.log(`\n=== ${spec}`);
    const r = spawnSync("node", [join(here, spec)], { stdio: "inherit", env: { ...process.env, UI_URL: ui, API_URL: api } });
    if (r.status !== 0) failed++;
  }
  console.log(failed === 0 ? `\nALL ${specs.length} E2E SPECS PASSED` : `\n${failed} of ${specs.length} E2E SPECS FAILED`);
  process.exitCode = failed === 0 ? 0 : 1;
} finally {
  stopAll();
  if (tmp) rmSync(tmp, { recursive: true, force: true });
}
