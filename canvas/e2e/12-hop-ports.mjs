// PC-152: the Workload form authors per-hop ports. A hop's port override reaches the real
// engine: pointing one hop at a port its security group does not admit blocks the journey at
// that hop with the server's own reason; clearing the override falls back to the journey's
// default port and the journey flows again. Also: the compliance option names v4.0.1.
import { open, check, done, mode, loadTemplate, baseline, flowOf } from "./lib.mjs";

const { page, close } = await open();
await loadTemplate(page, "three-tier-vpc");
await mode(page, "design");
if (await page.locator(".journey-hop-ports").count() === 0) await page.locator('button:has-text("Workload")').first().click();
await page.waitForTimeout(300);

const panels = page.locator(".journey-hop-ports");
check("every multi-hop journey offers a 'Ports along this journey' section", await panels.count() >= 3, String(await panels.count()));
check("PCI option is labelled v4.0.1", await page.locator("text=PCI DSS v4.0.1").count() > 0);

await mode(page, "simulate");
await baseline(page);
check("baseline: api flows end to end", (await flowOf(page, "api")) === "Flows end-to-end");

await mode(page, "design");
const api = page.locator(".journey-hop-ports").nth(1);
await api.locator("summary").click();
const inputs = api.locator('input[type="number"]');
const last = inputs.nth((await inputs.count()) - 1);
await last.fill("2222");
await mode(page, "simulate");
await baseline(page);
const blocked = await flowOf(page, "api");
check("a hop port its SG does not admit blocks the journey", /^Blocked at/.test(blocked), blocked);

await mode(page, "design");
await page.locator(".journey-hop-ports").nth(1).locator("summary").click();
const again = page.locator(".journey-hop-ports").nth(1).locator('input[type="number"]');
await again.nth((await again.count()) - 1).fill("");
await mode(page, "simulate");
await baseline(page);
check("clearing the override restores the journey default and it flows", (await flowOf(page, "api")) === "Flows end-to-end", await flowOf(page, "api"));

await close();
done("12-hop-ports");
