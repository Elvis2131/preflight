// PC-137: a security group authored on the canvas is evaluated by the server. With the
// allowing rule the first hop passes the SG step; removing it fails AT that step; with no SG
// attached the answer is not_assessable. (Replaces the one-off pc137_playwright.mjs.)
import { open, check, done, flowOf } from "./lib.mjs";

const { page, close } = await open();
const byLabel = (t) => page.locator(`xpath=//label[text()='${t}']/following-sibling::input[1]`);
await byLabel("name").fill("checkout-svc");
await byLabel("criticality (e.g. tier1)").fill("tier1");
await byLabel("data_classification").fill("PCI");
await byLabel("regions (comma-separated)").fill("eu-west-1");

async function drag(serviceID, group, x, y) {
  const card = page.locator(`[data-service="${serviceID}"]`);
  if (!await card.isVisible()) await page.locator(`[data-group="${group}"] > summary`).click();
  await card.hover();
  await page.mouse.down();
  const box = await page.locator(".react-flow__pane").boundingBox();
  await page.mouse.move(box.x + x, box.y + y, { steps: 10 });
  await page.mouse.up();
  await page.waitForTimeout(300);
}
await drag("aws_lb", "load_balancer", 300, 100);
await drag("aws_db_instance", "managed_database", 300, 300);
await drag("aws_security_group", "network_boundary", 300, 500);
const connect = async (from, to) => {
  const a = await page.locator(`.react-flow__node:has-text("${from}")`).first().boundingBox();
  const b = await page.locator(`.react-flow__node:has-text("${to}")`).first().boundingBox();
  await page.mouse.move(a.x + a.width / 2, a.y + a.height);
  await page.mouse.down();
  await page.mouse.move(b.x + b.width / 2, b.y, { steps: 10 });
  await page.mouse.up();
  await page.waitForTimeout(200);
};
await connect("Elastic Load Balancing", "Amazon RDS");
await connect("Elastic Load Balancing", "Security group"); // attaches the SG node to the LB (depends_on)

await page.getByTestId("canvas-json-toggle").click();
await page.waitForTimeout(200);
const doc = JSON.parse(await page.locator("pre").innerText());
await page.getByTestId("canvas-json-toggle").click();
const lbID = doc.nodes.find((n) => n.type === "load_balancer").id;

const node = (label) => page.locator(`.react-flow__node:has-text("${label}")`).first();
const inspector = page.locator("aside", { hasText: "Inspector" });
await node("Elastic Load Balancing").click({ force: true, position: { x: 10, y: 10 } });
await inspector.locator("select").first().selectOption("aws_lb");
await node("Security group").click({ force: true, position: { x: 10, y: 10 } });
await page.waitForTimeout(300);
const rule = async (port) => {
  await page.click("text=+ rule");
  const a = page.locator("aside", { hasText: "Inspector" });
  await a.locator('input[placeholder="protocol"]').last().fill("tcp");
  await a.locator('input[placeholder="from"]').last().fill(String(port));
  await a.locator('input[placeholder="to"]').last().fill(String(port));
  await a.locator('input[placeholder="cidr_blocks (comma-separated)"]').last().fill("0.0.0.0/0");
};
await rule(5432); // the allowing rule
await rule(443); // an unrelated one, so removing the first leaves a real SG with no match

const jf = (ph) => page.locator(`xpath=//input[@placeholder="${ph}" and not(ancestor::aside)]`).last();
await page.click("text=+ journey");
await jf("id").fill("checkout");
await jf("name").fill("Checkout");
await jf("path (e.g. internet, lb-1, db-1)").fill(`internet, ${lbID}`);
await jf("protocol").fill("tcp");
await jf("port").fill("5432");
await jf("criticality").fill("tier1");

await page.click("[data-mode=simulate]");
await page.waitForTimeout(500);
await page.click("text=Run baseline (no fault)");
await page.waitForTimeout(2200);
check("with the allowing rule, the journey passes the SG step", (await flowOf(page, "checkout")).startsWith("Flows end-to-end"));

await page.click("[data-mode=design]");
await page.waitForTimeout(500);
await node("Security group").click({ force: true, position: { x: 10, y: 10 } });
await page.locator("aside", { hasText: "Inspector" }).locator("text=×").first().click(); // remove the 5432 rule
await page.click("[data-mode=simulate]");
await page.waitForTimeout(500);
await page.click("text=Run baseline (no fault)");
await page.waitForTimeout(2200);
const blocked = await flowOf(page, "checkout");
check("removing the one allowing rule fails the journey AT the SG step", /Blocked at/.test(blocked) && /sg_dest_ingress/.test(blocked) && /no rule matched/.test(blocked), blocked);

await close();
done("01-sg-authoring");
