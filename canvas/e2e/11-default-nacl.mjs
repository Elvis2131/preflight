// PC-153: a default network ACL can be authored on the canvas. The palette offers it with its
// official icon; dropped inside a VPC it is tied to that VPC by a contained_in edge; the Inspector
// says what "default NACL" means and switches from "assumed default" to "replaces the default"
// once a rule is authored; the document carries service_id + rules. The engine-side resolution
// (declared, not assumed) is proved by the Go parity test (ingest/canvas_default_acl_test.go).
import { open, check, done, mode, loadTemplate, baseline, flowOf } from "./lib.mjs";

const { page, close } = await open();
const library = page.getByTestId("service-palette");

await page.getByRole("textbox", { name: "Search AWS services" }).fill("default network");
const card = library.locator('[data-service="aws_default_network_acl"]');
check("the palette offers the default network ACL", await card.isVisible());
check("it has its own official icon", (await card.locator("img").first().getAttribute("src")).endsWith("Res_Amazon-VPC_Network-Access-Control-List_48.svg"));
const iconOk = await card.locator("img").first().evaluate(async (img) => { const r = await fetch(img.src); return r.ok && (await r.text()).includes("<svg"); });
check("the icon loads as an SVG from the app", iconOk);
await page.getByRole("textbox", { name: "Search AWS services" }).fill("");

await loadTemplate(page, "three-tier-vpc");
const vpc = await page.locator('.react-flow__node[data-id="aws_vpc.main"]').boundingBox();
await page.locator(".canvas-stage").evaluate((stage, at) => {
  const dataTransfer = new DataTransfer();
  dataTransfer.setData("application/preflight-node-type", "network_boundary");
  dataTransfer.setData("application/preflight-service-id", "aws_default_network_acl");
  stage.dispatchEvent(new DragEvent("drop", { bubbles: true, cancelable: true, dataTransfer, clientX: at.x, clientY: at.y }));
}, { x: vpc.x + 40, y: vpc.y + 40 });
await page.waitForTimeout(500);

const readDoc = async () => {
  await page.getByTestId("canvas-json-toggle").click();
  const doc = JSON.parse(await page.locator("pre").innerText());
  await page.getByTestId("canvas-json-toggle").click();
  return doc;
};
let doc = await readDoc();
const dflt = doc.nodes.find((n) => n.service_id === "aws_default_network_acl");
check("the default NACL is on the canvas with its provider identity", Boolean(dflt));
check("dropped inside the VPC it is tied to it by a contained_in edge", doc.edges.some((e) => e.type === "contained_in" && e.from === dflt?.id && e.to === "aws_vpc.main"));
check("no rules are carried until some are authored", !dflt?.nacl_rules);

await page.locator(`.react-flow__node[data-id="${dflt.id}"]`).dispatchEvent("click");
await page.waitForTimeout(300);
const notice = page.getByTestId("default-nacl-notice");
check("the Inspector explains the default NACL", (await notice.innerText()).includes("not associated"));
check("with nothing authored it says the engine assumes AWS's default", (await notice.innerText()).includes("assumes AWS's default"));

await page.locator('[data-testid="nacl-editor"] button:has-text("+ rule")').click();
await page.locator('[data-testid="nacl-editor"] input[placeholder^="cidr_block"]').fill("0.0.0.0/0");
check("once a rule is authored it says the rules replace AWS's defaults", (await notice.innerText()).includes("replace"));
doc = await readDoc();
check("the rule is carried on the node", (doc.nodes.find((n) => n.id === dflt.id)?.nacl_rules ?? []).length === 1);

await mode(page, "simulate");
await baseline(page);
check("the design still assesses, and a journey through explicitly associated subnets still flows", (await flowOf(page, "web")) === "Flows end-to-end");
await close();
done("11-default-nacl");
