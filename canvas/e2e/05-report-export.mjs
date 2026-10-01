// PC-122/PC-123: Export PDF (headless Chromium) returns a real, date-normalized PDF; Re-price
// creates a NEW version priced against the active snapshot and leaves the old report unchanged.
// Needs a pricing snapshot: run.mjs seeds one (the test fixture's entries) before starting assessd.
import { open, check, done, mode, loadTemplate, baseline, sessionLabel } from "./lib.mjs";

const { page, close } = await open();
await loadTemplate(page, "three-tier-vpc");
await mode(page, "simulate");
await baseline(page);
await mode(page, "report");
await page.click("text=Generate / view report");
await page.waitForTimeout(2000);
check("the pricing snapshot is shown beside the cost", /e2e-fixture-snapshot/.test(await page.locator("body").innerText()));

const href = await page.locator('a:has(button:has-text("Export PDF"))').getAttribute("href");
const res = await fetch(href);
const pdf = Buffer.from(await res.arrayBuffer());
check("Export PDF returns a real PDF", res.status === 200 && res.headers.get("content-type") === "application/pdf" && pdf.slice(0, 5).toString() === "%PDF-", `${res.status} ${res.headers.get("content-type")}`);
check("the PDF's embedded dates are normalized", pdf.toString("latin1").includes("D:20000101000000"));
const html = await (await fetch(href.replace("format=pdf", "format=html"))).text();
check("the HTML export carries the mandatory cost disclaimer and the diagram", /not a bill/.test(html) && /<svg/.test(html));

const reportURL = href.replace(/\?format=pdf$/, "");
const v1 = await (await fetch(reportURL)).json();
await page.click("text=Re-price with latest snapshot");
await page.waitForTimeout(2500);
check("re-price creates a new version", /latest v2/.test(await sessionLabel(page)));
check("the old report is unchanged on read-back", JSON.stringify(v1) === JSON.stringify(await (await fetch(reportURL)).json()));
await close();
done("05-report-export");
