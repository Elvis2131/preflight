import { open, check, done, loadTemplate, mode, select, baseline, flowOf, fit } from "./lib.mjs";

const { page, close } = await open();
await loadTemplate(page, "three-tier-vpc");

const document = async () => {
  await page.getByTestId("canvas-json-toggle").click();
  const doc = JSON.parse(await page.locator("pre").innerText());
  await page.getByTestId("canvas-json-toggle").click();
  return doc;
};
const node = (id) => page.locator(`.react-flow__node[data-id="${id}"]`);
const size = (id) => node(id).evaluate((el) => ({ width: parseFloat(getComputedStyle(el).width), height: parseFloat(getComputedStyle(el).height) }));
const resize = async (id) => {
  await select(page, id);
  await fit(page);
  const before = await size(id);
  const handle = node(id).locator(".container-resize-handle.bottom.right");
  await handle.waitFor();
  const box = await handle.boundingBox();
  check("the minimap does not cover the selected container's resize controls", await page.locator(".react-flow__minimap").count() === 0);
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2 + 55, box.y + box.height / 2 + 25, { steps: 12 });
  await page.mouse.up();
  const after = await size(id);
  check(`${id} expands in both directions when its corner is dragged`, after.width > before.width + 20 && after.height > before.height + 20, `${JSON.stringify(before)} -> ${JSON.stringify(after)}`);
  check("the selected container explains how to expand it", (await node(id).innerText()).includes("Drag a corner or border to resize"));
  return after;
};
// Explicitly authored containers remain usable; template infrastructure stays hidden.
const drop = async (serviceID) => {
  await page.locator(".canvas-stage").evaluate((stage, serviceID) => {
    const rect = stage.getBoundingClientRect();
    const data = new DataTransfer();
    data.setData("application/preflight-node-type", "network_boundary");
    data.setData("application/preflight-service-id", serviceID);
    stage.dispatchEvent(new DragEvent("drop", { bubbles: true, dataTransfer: data, clientX: rect.x + 100, clientY: rect.y + 100 }));
  }, serviceID);
  const current = await document();
  return current.nodes.find((n) => n.service_id === serviceID && !n.id.startsWith("aws_")).id;
};
const vpcID = await drop("aws_vpc");
const subnetID = await drop("aws_subnet");
const original = await document();
const vpcSize = await resize(vpcID);
const subnetSize = await resize(subnetID);
check("resizing does not change resources, network assignments or the API document", JSON.stringify(await document()) === JSON.stringify(original));
await page.getByRole("button", { name: "Library", exact: true }).click();
await page.getByRole("button", { name: "Library", exact: true }).click();
check("container sizes survive panel layout changes", JSON.stringify(await size(vpcID)) === JSON.stringify(vpcSize) && JSON.stringify(await size(subnetID)) === JSON.stringify(subnetSize));
await mode(page, "simulate");
await select(page, subnetID);
check("read-only simulation mode has no resize controls", await page.locator(".container-resize-handle").count() === 0);
await mode(page, "design");
await select(page, subnetID);
check("resize controls return in Design", await node(subnetID).locator(".container-resize-handle").count() === 4);
// Remove unconfigured scratch containers before assessing the configured template.
for (const id of [subnetID, vpcID]) {
  await select(page, id);
  await page.keyboard.press("Backspace");
}
await mode(page, "simulate");
await baseline(page);
check("the configured template still simulates without an expanded graph view", await flowOf(page, "web") === "Flows end-to-end" && await flowOf(page, "api") === "Flows end-to-end" && await flowOf(page, "data") === "Flows end-to-end");
await close();
done("13-container-resizing");
