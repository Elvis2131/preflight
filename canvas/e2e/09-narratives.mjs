// PC-154: LLM narratives in the real app, against a real assessd talking to a STAND-IN reason worker
// (tests/e2e/stubreasond: no key, no network). Proves: nothing is generated until asked; narratives
// stream in labelled as LLM-written and separate from the findings; stored ones come back without
// another run; the report carries them as their own section; and an unavailable narrative layer is a
// banner, not an error, with the findings untouched.
import { writeFileSync } from "node:fs";
import { open, check, done, mode, loadTemplate, baseline } from "./lib.mjs";

const control = process.env.STUB_CONTROL;
if (!control) throw new Error("STUB_CONTROL is not set — run this spec through e2e/run.mjs");

const { page, close } = await open();
await loadTemplate(page, "three-tier-vpc");
await mode(page, "simulate");
await baseline(page); // version 1: assesses the current design

await mode(page, "analyze");
const panel = page.locator('[data-testid="narratives-panel"]');
await panel.waitFor();
check("the panel is labelled as LLM-written, with the notice", /LLM-WRITTEN/.test(await panel.innerText()) && /decides nothing/.test(await panel.locator('[data-testid="narratives-notice"]').innerText()));
check("nothing is generated until the architect asks", (await panel.locator('[data-testid="narrative"]').count()) === 0 && (await panel.locator('[data-testid="generate-narratives"]').count()) === 1);
const findingRows = await page.locator('[data-testid="timeline"] tbody tr').count();
check("the findings timeline is present and separate", findingRows > 0);

await panel.locator('[data-testid="generate-narratives"]').click();
await panel.locator('[data-testid="narratives-streaming"]').waitFor({ timeout: 10000 }).catch(() => undefined);
await page.waitForFunction(() => document.querySelectorAll('[data-testid="narrative"]').length >= 1, null, { timeout: 20000 });
await page.waitForFunction((n) => document.querySelectorAll('[data-testid="narrative"]').length >= n && !document.querySelector('[data-testid="narratives-streaming"]'), findingRows, { timeout: 30000 });
const count = await panel.locator('[data-testid="narrative"]').count();
check("one narrative per finding arrived", count === findingRows, `${count} narratives for ${findingRows} findings`);
check("each narrative names the evidence it cites and its llm_reasoned provenance", /cites: .+ · provenance: llm_reasoned/.test(await panel.locator('[data-testid="narrative"]').first().innerText()));
check("the generate button is gone once complete", (await panel.locator('[data-testid="generate-narratives"]').count()) === 0);
check("the findings timeline is untouched by narratives", (await page.locator('[data-testid="timeline"] tbody tr').count()) === findingRows);

// Stored narratives come back without another run.
await mode(page, "design");
await mode(page, "analyze");
await page.locator('[data-testid="narratives-panel"]').waitFor();
await page.waitForFunction((n) => document.querySelectorAll('[data-testid="narrative"]').length === n, count, { timeout: 10000 });
check("stored narratives show again with no click and no new run", (await page.locator('[data-testid="generate-narratives"]').count()) === 0);

// The report carries them as their own section.
await mode(page, "report");
await page.click("text=Generate / view report");
await page.waitForTimeout(2500);
const section = page.locator('[data-testid="report-narratives"]');
check("the report has a separate, labelled narratives section", (await section.count()) === 1 && /LLM-written/.test(await section.innerText()));

// An unavailable narrative layer is a banner, not an error, on a version with nothing stored.
writeFileSync(control, "nokey");
await mode(page, "design");
await mode(page, "simulate");
await baseline(page); // version 2
await mode(page, "analyze");
const panel2 = page.locator('[data-testid="narratives-panel"]');
await panel2.waitFor();
await page.waitForTimeout(600);
await panel2.locator('[data-testid="generate-narratives"]').click();
const banner = panel2.locator('[data-testid="narratives-degraded"]');
await banner.waitFor({ timeout: 15000 });
check("a keyless worker is a degraded banner naming why", /no API key/.test(await banner.innerText()));
check("the banner says the findings are unaffected", /unaffected/.test(await banner.innerText()));
check("no narratives are shown for it", (await panel2.locator('[data-testid="narrative"]').count()) === 0);
check("the findings timeline is still there", (await page.locator('[data-testid="timeline"] tbody tr').count()) > 0);
await close();
done("09-narratives");
