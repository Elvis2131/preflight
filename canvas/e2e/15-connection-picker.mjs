import { open, check, done, mode } from "./lib.mjs";

const { page, close } = await open();
async function drop(nodeType, serviceID, x, y) {
  await page.locator(".canvas-stage").evaluate((stage, item) => {
    const rect = stage.getBoundingClientRect();
    const data = new DataTransfer();
    data.setData("application/preflight-node-type", item.nodeType);
    data.setData("application/preflight-service-id", item.serviceID);
    stage.dispatchEvent(new DragEvent("drop", { bubbles: true, dataTransfer: data, clientX: rect.x + item.x, clientY: rect.y + item.y }));
  }, { nodeType, serviceID, x, y });
  await page.waitForTimeout(350);
}
await drop("container_workload", "aws_eks_cluster", 180, 180);
await drop("managed_database", "aws_db_instance", 380, 430);
await page.locator(".react-flow__pane").click({ position: { x: 12, y: 12 } });
await page.waitForTimeout(500);
await page.getByTestId("arrange-canvas").click();
await page.waitForTimeout(500);
await page.getByTestId("canvas-json-toggle").click();
const read = async () => JSON.parse(await page.locator("pre").innerText());
const initial = await read();
const source = initial.nodes.find((node) => node.service_id === "aws_eks_cluster").id;
const target = initial.nodes.find((node) => node.service_id === "aws_db_instance").id;
async function connect(from, to) {
  const a = await page.locator(`.react-flow__node[data-id="${from}"] .react-flow__handle.source`).boundingBox();
  const b = await page.locator(`.react-flow__node[data-id="${to}"] .react-flow__handle.target`).boundingBox();
  await page.mouse.move(a.x + a.width / 2, a.y + a.height / 2);
  await page.mouse.down();
  await page.mouse.move(b.x + b.width / 2, b.y + b.height / 2, { steps: 15 });
  await page.mouse.up();
  try { await page.getByRole("dialog").waitFor({ timeout: 3000 }); }
  catch (error) { await page.screenshot({ path: "/tmp/preflight-connection-debug.png" }); console.log("Handle positions", a, b); throw error; }
}
const choose = (type) => page.getByRole("dialog").locator(`[data-connection-type="${type}"]`).click();
await connect(source, target);
check("finishing a drag opens the connection picker", await page.getByRole("heading", { name: "Choose the connection type" }).isVisible());
check("the picker names both endpoints", (await page.locator(".connection-endpoints").innerText()).includes("Amazon EKS") && (await page.locator(".connection-endpoints").innerText()).includes("Amazon RDS"));
check("an unconfirmed connection never enters the saved document", (await read()).edges.length === 0);
const pickerBox = await page.getByRole("dialog").boundingBox();
check("all connection options fit within the viewport", pickerBox.y >= 0 && pickerBox.y + pickerBox.height <= page.viewportSize().height);
await page.screenshot({ path: "/tmp/preflight-connection-picker.png" });
await page.setViewportSize({ width: 600, height: 800 });
await page.waitForTimeout(200);
const compactBox = await page.getByRole("dialog").boundingBox();
check("the popup stays usable when the viewport shrinks", compactBox.x >= 0 && compactBox.x + compactBox.width <= 600 && compactBox.y + compactBox.height <= 800);
await page.setViewportSize({ width: 1800, height: 1050 });
await page.waitForTimeout(300);
await page.keyboard.press("Escape");
check("Escape cancels without saving a default type", await page.getByRole("dialog").count() === 0 && (await read()).edges.length === 0);
await connect(source, target);
await choose("routes_to");
let doc = await read();
const original = doc.edges[0];
check("choosing a type creates exactly one correctly directed connection", doc.edges.length === 1 && original.type === "routes_to" && original.from === source && original.to === target);
await page.locator(`.react-flow__edge[data-id="${original.id}"] .react-flow__edge-textwrapper`).click();
await page.getByRole("dialog").waitFor();
check("clicking a connector shows its current type", await page.locator('[data-connection-type="routes_to"]').getAttribute("aria-pressed") === "true");
await choose("reads/writes");
doc = await read();
check("editing updates the existing connection without changing its ID or endpoints", doc.edges.length === 1 && doc.edges[0].id === original.id && doc.edges[0].type === "reads/writes" && doc.edges[0].from === source && doc.edges[0].to === target);
await connect(source, target);
await choose("depends_on");
check("drawing the same connection edits rather than duplicates it", (await read()).edges.length === 1 && (await read()).edges[0].id === original.id);
await connect(target, source);
await page.getByRole("button", { name: "Cancel connection" }).click();
check("cancelling a new connection preserves existing connections", (await read()).edges.length === 1);
await page.locator(`.react-flow__edge[data-id="${original.id}"]`).focus();
await page.keyboard.press("Enter");
await page.getByRole("dialog").waitFor();
check("focused connections can be edited with the keyboard", await page.locator('[data-connection-type="depends_on"]').getAttribute("aria-pressed") === "true");
await page.keyboard.press("Escape");
await page.getByTestId("canvas-json-toggle").click();
await mode(page, "simulate");
await page.locator(`.react-flow__edge[data-id="${original.id}"]`).dispatchEvent("click");
check("simulation cannot change connection types", await page.getByRole("dialog").count() === 0);
await mode(page, "design");
await page.getByRole("button", { name: "Switch to dark mode" }).click();
await page.locator(`.react-flow__edge[data-id="${original.id}"]`).dispatchEvent("click");
await page.getByRole("dialog").waitFor();
await page.screenshot({ path: "/tmp/preflight-connection-picker-dark.png" });
await page.keyboard.press("Escape");
await close();
done("15-connection-picker");
