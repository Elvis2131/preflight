// Saved network designs load through the ordinary API, preserve the full graph,
// and distinguish checked traffic paths from unmodelled architecture intent.
import { open, check, done, loadTemplate, mode, select, baseline, flowOf, API } from "./lib.mjs";

const { page, close } = await open();
const listing = await (await fetch(`${API}/templates`)).json();
check("both network templates are listed by the backend", ["simple-aws-network", "enterprise-aws-network"].every((id) => listing.some((t) => t.id === id)));
const document = async () => {
  await page.getByTestId("canvas-json-toggle").click();
  const doc = JSON.parse(await page.locator("pre").innerText());
  await page.getByTestId("canvas-json-toggle").click();
  return doc;
};
const positions = async () => page.locator(".react-flow__node-golden").evaluateAll((els) => Object.fromEntries(els.map((el) => [el.getAttribute("data-id"), el.style.transform])));
await loadTemplate(page, "simple-aws-network");
check("the simple design opens as four services and their internet client", await page.locator(".react-flow__node-golden").count() === 5);
check("the client has a distinct DNS lookup connection to Route 53", await page.locator('.react-flow__edge[data-id="traffic-display:dns-lookup"]').count() === 1 && (await page.locator('.react-flow__edge[data-id="traffic-display:dns-lookup"]').textContent()).includes("DNS lookup") && (await page.locator('.react-flow__edge[data-id="traffic-display:https-request"]').textContent()).includes("HTTPS request"));
check("the loaded template is identified in the toolbar", (await page.getByTestId("loaded-template").innerText()).includes("Simple AWS network"));
const simple = await document();
check("the simple design includes its 22 actual resources", simple.nodes.length === 22 && simple.nodes.some((n) => n.service_id === "aws_instance") && simple.nodes.some((n) => n.service_id === "aws_iam_role"));
check("database subnet routing is local only", simple.nodes.find((n) => n.id === "aws_route_table.simple_data").routes === undefined);
await select(page, "aws_instance.simple");
check("EC2 access and network assignments are available in the inspector", (await page.getByTestId("configuration-iam_role").innerText()).includes("EC2 application role") && (await page.getByTestId("configuration-security_groups").innerText()).includes("App · only from entry"));
await mode(page, "simulate");
await baseline(page);
check("simple HTTPS ingress is assessed by the real backend", await flowOf(page, "simple_web") === "Flows end-to-end");
const ec2 = await flowOf(page, "simple_app");
check("unmodelled EC2 request behavior is disclosed", /capability_check/.test(ec2), ec2);
await mode(page, "design");
await loadTemplate(page, "enterprise-aws-network");
check("the enterprise blueprint opens as thirteen services and their internet client", await page.locator(".react-flow__node-golden").count() === 14);
const before = await positions();
const enterprise = await document();
check("all 67 enterprise resources and their intended policy are saved", enterprise.nodes.length === 67 && enterprise.nodes.find((n) => n.service_id === "aws_ec2_transit_gateway").capability.design_route_tables.includes("blackhole"));
check("three VPC boundaries and four zonal NAT gateways are present", enterprise.nodes.filter((n) => n.service_id === "aws_vpc").length === 3 && enterprise.nodes.filter((n) => n.service_id === "aws_nat_gateway").length === 4);
check("network resources stay configured without an expanded graph view", await page.getByTestId("infrastructure-toggle").count() === 0 && JSON.stringify(await document()) === JSON.stringify(enterprise));
check("the service layout remains stable", JSON.stringify(await positions()) === JSON.stringify(before));
await select(page, "aws_ec2_transit_gateway.hub");
await page.getByTestId("architecture-notes").locator("summary").click();
check("the transit policy can be inspected by selecting its core service", (await page.getByTestId("architecture-notes").innerText()).includes("nonproduction") && (await page.getByTestId("architecture-notes").innerText()).includes("blackhole"));
await mode(page, "simulate");
await baseline(page);
check("enterprise ingress and internal application hops are assessed", await flowOf(page, "production_web") === "Flows end-to-end" && await flowOf(page, "production_api") === "Flows end-to-end");
const data = await flowOf(page, "production_data");
const hybrid = await flowOf(page, "hybrid_dns");
check("database isolation is retained despite the current route model limit", /route_selection/.test(data), data);
check("hybrid VPN behavior remains unknown rather than falsely passing", /capability_check/.test(hybrid), hybrid);
await close();
done("09-network-templates");
