// Shared helpers for the end-to-end specs. Every spec drives the REAL workspace in a real
// (headless Chromium) browser against a REAL assessd — nothing is mocked — and asserts: a
// failed check prints what was expected and exits non-zero, so a spec is evidence, not a
// screenshot to eyeball.
import { chromium } from "playwright-core";

export const UI = process.env.UI_URL ?? "http://localhost:5183";
export const API = process.env.API_URL ?? "http://localhost:8099";

let failures = 0;
export function check(name, ok, detail = "") {
  if (ok) console.log(`  ok   ${name}`);
  else {
    failures++;
    console.log(`  FAIL ${name}${detail ? " — " + detail : ""}`);
  }
}
export function done(spec) {
  console.log(failures === 0 ? `PASS ${spec}` : `FAILED ${spec}: ${failures} check(s)`);
  process.exit(failures === 0 ? 0 : 1);
}

export async function open() {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1800, height: 1050 } });
  page.on("pageerror", (e) => check("no uncaught page error", false, e.message));
  page.on("dialog", (d) => d.accept());
  await page.goto(UI);
  await page.waitForTimeout(900);
  return { browser, page, close: () => browser.close() };
}

export const mode = async (page, m) => {
  await page.locator(`[data-mode="${m}"]`).click();
  await page.waitForTimeout(600);
};
// Select a node by its stable id. The click event is dispatched straight to the node's own
// element, so an edge's SVG path or a label drawn over it can never intercept it (a real mouse
// click at a point under an edge selects the edge instead).
export const select = async (page, id) => {
  await page.locator(`.react-flow__node[data-id="${id}"]`).dispatchEvent("click");
  await page.waitForTimeout(300);
};
export const loadTemplate = async (page, id) => {
  await page.locator('[data-testid="template-picker"]').selectOption(id);
  await page.waitForTimeout(1200);
};
export const fit = async (page) => {
  await page.locator(".react-flow__controls-fitview").click();
  await page.waitForTimeout(500);
};
export const baseline = async (page) => {
  await page.click("text=Run baseline (no fault)");
  await page.waitForTimeout(2200);
};
// The flow line the journey panel shows for one journey, exactly as the server returned it.
export const flowOf = async (page, journey) => {
  const panel = page.locator("text=Journey flow & utilization").locator("..");
  await panel.locator("select").selectOption(journey);
  await page.waitForTimeout(250);
  const lines = (await panel.innerText()).split("\n").map((l) => l.trim()).filter(Boolean);
  const i = lines.findIndex((l) => /^(Flows end-to-end|Blocked at)/.test(l));
  return i >= 0 ? lines[i] : "";
};
export const sessionLabel = async (page) => (await page.locator('[data-testid="session-label"]').innerText()).trim();
