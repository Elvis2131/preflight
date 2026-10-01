// PC-138/PC-139 (+ PC-113): a canvas-built multi-hop design flows end to end through every SG,
// NACL and route-table step, and editing exactly one control through the Inspector blocks the
// journey at exactly that step with the server's own reason.
import { open, check, done, mode, select, loadTemplate, baseline, flowOf } from "./lib.mjs";

const { page, close } = await open();
await loadTemplate(page, "three-tier-vpc");
await mode(page, "simulate");
await baseline(page);
for (const j of ["web", "api", "data"]) check(`baseline: journey ${j} flows end to end`, (await flowOf(page, j)) === "Flows end-to-end");

await mode(page, "design");
await select(page, "aws_route_table.private");
const targets = await page.locator('[data-testid="routes-editor"] select option').allInnerTexts();
check("the routes editor offers only IGW/NAT targets", targets.every((t) => t.startsWith("—") || /aws_(nat_gateway|internet_gateway)/.test(t)), targets.join(" | "));
await page.locator('[data-testid="routes-editor"] button:has-text("×")').first().click();
await mode(page, "simulate");
await baseline(page);
const noRoute = await flowOf(page, "api");
check("removing the private route blocks api at route_selection", /route_selection/.test(noRoute), noRoute);

await mode(page, "design");
await select(page, "aws_route_table.private");
await page.locator('[data-testid="routes-editor"] button:has-text("+ route")').click();
await page.locator('[data-testid="routes-editor"] input').first().fill("0.0.0.0/0");
await page.locator('[data-testid="routes-editor"] select').first().selectOption("aws_nat_gateway.nat_a");
await select(page, "aws_network_acl.default");
check("the NACL editor shows the engine-owned catch-all as read-only", (await page.locator('[data-testid="nacl-catchall"]').innerText()).includes("deny all"));
await page.locator('[data-testid="nacl-editor"] select').nth(1).selectOption("deny");
await mode(page, "simulate");
await baseline(page);
const denied = await flowOf(page, "api");
check("restoring the route and denying the NACL blocks api at the NACL step, rule named", /nacl_dest_ingress/.test(denied) && /rule 100 \(DENY\)/.test(denied), denied);

// out-of-range rule number is flagged in the editor (the server decides, and has its own test)
await mode(page, "design");
await select(page, "aws_network_acl.default");
await page.locator('[data-testid="nacl-editor"] input[type=number]').first().fill("40000");
check("an out-of-range NACL rule number is flagged", (await page.locator('[data-testid="nacl-editor"]').innerText()).includes("32767-65535"));
await close();
done("02-network-controls");
