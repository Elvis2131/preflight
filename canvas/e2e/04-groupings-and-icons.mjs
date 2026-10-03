// The service-only canvas retains official AWS icons and omits expanded graph groupings.
import { open, check, done, mode, loadTemplate } from "./lib.mjs";

const { page, close } = await open();
// palette
const items = await page.locator('[data-testid="service-palette"] [data-service]').evaluateAll((els) => els.map((e) => ({ icon: !!e.querySelector("img") })));
check("every AWS service in the palette has an official service, resource or category icon", items.length > 250 && items.every((i) => i.icon));
const grouped = await page.locator('[data-testid="service-palette"]').evaluate((root) => {
  const group = (type) => Array.from(root.querySelectorAll(`[data-group="${type}"] [data-service]`)).map((e) => e.getAttribute("data-service"));
  return { compute: group("compute"), network: group("network_boundary") };
});
check("AWS services are grouped under their core components", grouped.compute.includes("aws_lambda_function") && grouped.network.includes("aws_vpc") && grouped.network.includes("aws_security_group"), JSON.stringify(grouped));

await loadTemplate(page, "three-tier-vpc");
check("the canvas displays services without expanded infrastructure groupings", await page.locator("[data-grouping]").count() === 0 && await page.locator(".react-flow__node-golden").count() === 5);
const icons = await page.locator(".react-flow__node img").evaluateAll((els) => els.map((e) => e.getAttribute("src")));
check("visible architecture services retain official AWS icons", icons.length === 5 && icons.every((src) => src.includes("/aws-icons/")));
await mode(page, "simulate");
check("simulation retains the service canvas", await page.locator(".react-flow__node-golden").count() === 5 && await page.getByTestId("infrastructure-toggle").count() === 0);
await close();
done("04-groupings-and-icons");
