// PC-105 + PC-109: Region/AZ groupings and the public/private badge are DERIVED by the server
// and follow the model; groupings are read-only and never serialized; official icons and the
// service palette render.
import { open, check, done, mode, select, loadTemplate, fit } from "./lib.mjs";

const { page, close } = await open();
const badge = (id) => page.locator(`.react-flow__node[data-id="${id}"] [data-testid="subnet-badge"]`).getAttribute("data-visibility").catch(() => "(none)");
const groups = async () => (await page.locator("[data-grouping]").evaluateAll((els) => els.map((e) => `${e.getAttribute("data-grouping")}:${e.getAttribute("data-label")}`))).sort();

// palette
const items = await page.locator('[data-testid="service-palette"] [data-service]').evaluateAll((els) => els.map((e) => ({ icon: !!e.querySelector("img") })));
check("the service palette lists registry services, some with official icons", items.length > 15 && items.some((i) => i.icon) && items.some((i) => !i.icon));

await loadTemplate(page, "three-tier-vpc");
await page.waitForTimeout(800);
await fit(page);
check("a public subnet is badged public (route to the internet gateway)", (await badge("aws_subnet.public_a")) === "public");
check("a private subnet is badged private", (await badge("aws_subnet.private_a")) === "private");
check("Region and AZ groupings are drawn", JSON.stringify(await groups()) === JSON.stringify(["az:eu-west-1a", "az:eu-west-1b", "region:eu-west-1"]), JSON.stringify(await groups()));
check("groupings are not interactive", (await page.locator("[data-grouping]").first().evaluate((e) => getComputedStyle(e).pointerEvents)) === "none");
const icons = await page.locator(".react-flow__node img").evaluateAll((els) => els.map((e) => e.getAttribute("src").split("/").pop()));
check("official icons are drawn on services and groups", ["Amazon-RDS", "Elastic-Load", "Internet-Gateway", "Public-subnet", "Private-subnet", "Virtual-private"].every((k) => icons.some((s) => s.includes(k))));

await page.click("text=Show CanvasDocument JSON");
await page.waitForTimeout(300);
const doc = JSON.parse(await page.locator("pre").innerText());
check("no grouping node leaks into the CanvasDocument", doc.nodes.every((n) => !n.id.startsWith("grouping:")));
await page.click("text=Hide CanvasDocument JSON");

await select(page, "aws_route_table.public");
await page.locator('[data-testid="routes-editor"] button:has-text("×")').first().click();
await page.waitForTimeout(1500);
check("remove the public route table's route: the badge becomes not_assessable (never private)", (await badge("aws_subnet.public_a")) === "not_assessable");

await select(page, "aws_subnet.private_b");
await page.locator('[data-testid="placement-editor"] input[placeholder="e.g. eu-west-1a"]').fill("eu-west-1c");
await page.waitForTimeout(1500);
check("set a subnet's AZ to eu-west-1c: a third AZ grouping appears", (await groups()).includes("az:eu-west-1c"));

await mode(page, "simulate");
check("the groupings still draw in Simulate mode", (await groups()).length >= 3);
await close();
done("04-groupings-and-icons");
