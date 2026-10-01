// Exercise service authoring and toolbar controls against the real app and API.
import { open, check, done, loadTemplate } from "./lib.mjs";

const { page, close } = await open();
const library = page.getByTestId("service-palette");
check("all core headings are non-draggable", await library.locator("[data-component]").evaluateAll((els) => els.length === 11 && els.every((e) => !e.draggable)));
check("only AWS service cards are draggable", await library.locator('[draggable="true"]').evaluateAll((els) => els.length > 250 && els.every((e) => e.hasAttribute("data-service"))));
check("every service has an official AWS icon", await library.locator("[data-service]").evaluateAll((els) => els.every((e) => e.querySelector('img[data-testid="aws-icon"]') && !e.querySelector('[data-testid="service-mark"]'))));
const iconFailures = await library.evaluate(async (el) => {
  const urls = [...new Set(Array.from(el.querySelectorAll("img")).map((img) => img.src))];
  const failed = [];
  for (let i = 0; i < urls.length; i += 12) {
    const batch = await Promise.all(urls.slice(i, i + 12).map(async (url) => ({ url, response: await fetch(url) })));
    for (const { url, response } of batch) if (!response.ok || !(await response.text()).includes("<svg")) failed.push(url);
  }
  return failed;
});
check("all palette icons load as SVGs from the app", iconFailures.length === 0, iconFailures.join(", "));
check("category fallback icons identify themselves in the tooltip", await library.locator('img[data-icon-kind="category"]').evaluateAll((els) => els.length > 0 && els.every((e) => e.title.includes("no dedicated icon"))));
await library.locator('[data-component="dns"]').click();
check("DNS opens as a section with draggable AWS services", await library.locator('[data-service="aws_route53_record"]').isVisible());
await library.locator('[data-component="dns"]').click();

const search = page.getByRole("textbox", { name: "Search AWS services" });
await search.fill("EC2");
check("search finds EC2 under Compute", await library.locator('[data-group="compute"] [data-service="aws_instance"]').isVisible());
check("EC2 uses its own official service icon", (await library.locator('[data-service="aws_instance"] img').getAttribute("src")).endsWith("Arch_Amazon-EC2_48.svg"));
await search.fill("Bedrock");
check("search opens supporting categories to reveal matching services", await library.locator('[data-service="aws_service:bedrock"]').isVisible());
await search.fill("no-such-aws-service");
check("search explains an empty result", await page.getByText("No matching components or AWS services.").isVisible());
await search.fill("");

const drop = async (nodeType, serviceID, offset) => {
  await page.locator(".canvas-stage").evaluate((stage, { nodeType, serviceID, offset }) => {
    const rect = stage.getBoundingClientRect();
    const dataTransfer = new DataTransfer();
    dataTransfer.setData("application/preflight-node-type", nodeType);
    if (serviceID) dataTransfer.setData("application/preflight-service-id", serviceID);
    stage.dispatchEvent(new DragEvent("drop", { bubbles: true, cancelable: true, dataTransfer, clientX: rect.x + offset, clientY: rect.y + 180 }));
  }, { nodeType, serviceID, offset });
};
await drop("dns", "", 160);
check("dropping a core type without an AWS service creates nothing", await page.locator(".react-flow__node-golden").count() === 0);
await drop("compute", "aws_instance", 180);
await page.locator('.react-flow__node-golden').first().waitFor();
check("new AWS services can be placed on the canvas", await page.locator(".react-flow__node-golden").count() === 1);
check("the service icon is carried onto the canvas", (await page.locator('.react-flow__node-golden img').first().getAttribute("src")).endsWith("Arch_Amazon-EC2_48.svg"));
await page.locator('.react-flow__node-golden').first().dispatchEvent("click");
check("a service with no backend model explains its assessment limits", await page.getByText("Available for architecture design.", { exact: false }).isVisible());
await drop("external_dependency", "aws_service:bedrock", 360);
await page.getByTestId("canvas-json-toggle").click();
const document = JSON.parse(await page.locator("pre").innerText());
check("real provider identities are preserved", document.nodes.some((n) => n.service_id === "aws_instance"));
check("catalogue-only products keep their labels without invented provider IDs", document.nodes.some((n) => n.label === "Amazon Bedrock" && n.type === "external_dependency" && !n.service_id));
await page.getByTestId("canvas-json-toggle").click();

await page.getByTestId("connection-type").selectOption("routes_to");
check("connection controls have friendly labels and contextual guidance", await page.getByTestId("connection-type").locator("option:checked").innerText() === "Routes traffic to" && (await page.locator("#connection-guide").innerText()).includes("sending traffic"));
const toolbar = page.getByRole("toolbar", { name: "Architecture tools" });
await toolbar.getByRole("button", { name: "Workload", exact: true }).click();
check("Workload toggles the requirements panel", await page.locator(".workload-panel").count() === 0);
await toolbar.getByRole("button", { name: "Workload", exact: true }).click();
check("Workload can be reopened", await page.locator(".workload-panel").count() === 1);

await loadTemplate(page, "three-tier-vpc");
check("the template picker still loads a real architecture", await page.locator('.react-flow__node[data-id="aws_vpc.main"]').count() === 1);
await page.setViewportSize({ width: 1024, height: 800 });
check("the toolbar fits the smallest supported desktop width", await toolbar.evaluate((el) => el.scrollWidth <= el.clientWidth));
await toolbar.getByRole("button", { name: "Test design" }).click();
check("Test design opens Simulate with the design preserved", await page.locator('[data-mode="simulate"]').evaluate((e) => e.classList.contains("active")) && await page.locator('.react-flow__node[data-id="aws_vpc.main"]').count() === 1);
check("authoring controls are hidden during simulation", await page.getByTestId("connection-type").count() === 0);

await close();
done("06-service-library-and-toolbar");
