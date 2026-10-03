// Drive the actual backend, then verify direction, pacing, stop markers and that
// display-only traffic cards/edges never enter the saved assessment input.
import { open, check, done, loadTemplate, mode, select } from "./lib.mjs";

const { page, close } = await open();
const assessments = [];
page.on("request", (r) => { if (r.method() === "POST" && /\/sessions\/[^/]+\/canvas$/.test(new URL(r.url()).pathname)) assessments.push(r.postDataJSON()); });
const document = async () => {
  await page.getByTestId("canvas-json-toggle").click();
  const doc = JSON.parse(await page.locator("pre").innerText());
  await page.getByTestId("canvas-json-toggle").click();
  return doc;
};
const follow = async () => {
  const response = page.waitForResponse((r) => r.url().endsWith("/simulate") && r.request().method() === "POST");
  await page.getByTestId("follow-traffic").click();
  const result = await (await response).json();
  await page.getByTestId("traffic-player").waitFor();
  return result;
};
const packetDistance = () => page.getByTestId("traffic-packet").first().evaluate((el) => getComputedStyle(el).offsetDistance);

await loadTemplate(page, "three-tier-vpc");
const original = await document();
const result = await follow();
await page.getByTestId("traffic-packet").first().waitFor();
check("Follow traffic assesses the design and opens the selected journey", result.flow_detail.find((f) => f.JourneyID === "web").Flows && await page.locator('[data-mode="simulate"]').getAttribute("class").then((s) => s.includes("active")));
check("internet ingress has a visible source and directional edge", await page.locator('.react-flow__node[data-id="traffic-display:internet"]').count() === 1 && await page.locator('.react-flow__edge path[marker-end]').count() > 0);
await page.getByLabel("Playback speed").selectOption("0.5");
const moving1 = await packetDistance();
await page.waitForTimeout(250);
check("a packet actually travels along the edge", await packetDistance() !== moving1);
await page.getByTestId("traffic-play").click();
await page.waitForFunction(() => document.querySelector('[data-testid="traffic-packet"]') && getComputedStyle(document.querySelector('[data-testid="traffic-packet"]')).animationPlayState === "paused");
await page.waitForTimeout(60);
const paused = await packetDistance();
await page.waitForTimeout(300);
const afterPause = await packetDistance();
const playLabel = await page.getByTestId("traffic-play").innerText();
check("Pause freezes the packet", afterPause === paused && playLabel === "Play", `${paused} -> ${afterPause}; ${playLabel}`);
await page.getByRole("button", { name: "Next hop", exact: true }).click();
check("single-hop playback completes with no remaining moving packets", (await page.getByTestId("traffic-progress").innerText()).includes("Replay complete") && await page.getByTestId("traffic-packet").count() === 0);
await page.getByRole("button", { name: "Replay", exact: true }).click();
await page.getByTestId("traffic-packet").first().waitFor();
check("Replay restarts the actual hop", (await page.getByTestId("traffic-progress").innerText()).includes("Hop 1 of 1"));
await mode(page, "design");
check("playback leaves the complete saved canvas document unchanged", JSON.stringify(await document()) === JSON.stringify(original));
check("the API input contains no virtual internet card, traffic edge or playback data", JSON.stringify(assessments[0].canvas) === JSON.stringify(original));

// A declared two-hop journey: first hop allowed, second stopped at the app SG.
const workloadButton = page.getByRole("toolbar", { name: "Architecture tools" }).getByRole("button", { name: "Workload", exact: true });
if (await workloadButton.getAttribute("aria-pressed") === "false") await workloadButton.click();
await page.getByPlaceholder("path (e.g. internet, lb-1, db-1)").first().fill("internet, aws_lb.web, aws_eks_cluster.app");
await follow();
await page.getByTestId("traffic-stop-marker").waitFor({ timeout: 10000 });
check("playback advances in order and stops at the backend-rejected hop", (await page.getByTestId("traffic-progress").innerText()).includes("Stopped at hop 2 of 2") && (await page.getByTestId("traffic-player").innerText()).includes("sg_dest_ingress") && await page.getByTestId("traffic-packet").count() === 0 && await page.getByTestId("traffic-play").innerText() === "Play");

// Restore actual access: a fresh assessment must replace the old stopped result.
await mode(page, "design");
await select(page, "aws_eks_cluster.app");
const groups = page.getByTestId("configuration-security_groups");
await groups.getByRole("button", { name: "Edit App security group", exact: true }).click();
await groups.getByRole("spinbutton", { name: "From port", exact: true }).first().fill("443");
await groups.getByRole("spinbutton", { name: "To port", exact: true }).first().fill("443");
const restored = await follow();
check("a fresh replay uses the newly assessed rules", restored.flow_detail.find((f) => f.JourneyID === "web").Flows);
await page.getByLabel("Playback speed").selectOption("2");
await page.getByRole("button", { name: "Replay", exact: true }).click();
await page.getByTestId("traffic-progress").filter({ hasText: "Hop 2 of 2" }).waitFor();
check("the second allowed hop has a moving packet and numbered services", await page.getByTestId("traffic-packet").count() === 1 && await page.locator('[data-traffic-step="2"]').count() === 2);
await page.getByTestId("traffic-progress").filter({ hasText: "Replay complete" }).waitFor();

// Reduced-motion users can still follow and step through exactly the same result.
await page.emulateMedia({ reducedMotion: "reduce" });
await page.getByRole("button", { name: "Replay", exact: true }).click();
check("reduced motion starts paused and explains manual stepping", await page.getByTestId("traffic-play").isDisabled() && (await page.getByTestId("traffic-player").innerText()).includes("Reduced motion") && await page.getByTestId("traffic-packet").first().evaluate((el) => getComputedStyle(el).animationName) === "none");
await page.getByRole("button", { name: "Next hop", exact: true }).click();
check("Next hop remains usable with reduced motion", (await page.getByTestId("traffic-progress").innerText()).includes("Hop 2 of 2"));
await page.emulateMedia({ reducedMotion: "no-preference" });

await mode(page, "design");
await loadTemplate(page, "simple-aws-network");
const simpleBefore = await document();
const simpleResult = await follow();
await page.getByLabel("Playback speed").selectOption("0.5");
check("the simple request starts with the client DNS lookup", (await page.getByTestId("traffic-progress").innerText()).includes("DNS lookup 1 of 3") && await page.locator('[data-testid="traffic-packet"][data-traffic-kind="dns"]').count() === 1 && (await page.getByTestId("traffic-player").innerText()).includes("DNS resolution is not assessed"));
check("DNS remains a presentation step, not a fabricated backend hop", simpleResult.flow_detail.find((f) => f.JourneyID === "simple_request").Hops.every((h) => h.From !== "aws_route53_record.simple" && h.To !== "aws_route53_record.simple") && JSON.stringify(assessments.at(-1).canvas) === JSON.stringify(simpleBefore));
const summary = await page.getByTestId("simulation-status").innerText();
check("baseline status explains missing capacity without exposing internal instructions", summary.includes("Baseline · no fault injected") && summary.includes("Application capacity is not set") && !/CLAUDE\.md|capacity_unknown|workload\.yaml|severed_paths/.test(summary));
await page.getByRole("button", { name: "Show hop 2", exact: true }).click();
check("after DNS, HTTPS goes directly from client to ALB", (await page.getByTestId("traffic-progress").innerText()).includes("Internet clients → HTTPS entry point") && await page.locator('[data-testid="traffic-packet"][data-traffic-kind="assessed"]').count() === 1 && await page.locator('[data-testid="traffic-packet"][data-traffic-kind="dns"]').count() === 0);
await page.getByLabel("Traffic journey", { exact: true }).selectOption("simple_app");
check("changing journeys discards the previous frame and preserves unknown results", (await page.getByTestId("traffic-progress").innerText()).includes("Stopped at hop 1 of 1") && (await page.getByTestId("traffic-player").innerText()).includes("capability_check") && await page.getByTestId("traffic-stop-marker").count() === 1 && await page.getByTestId("traffic-packet").count() === 0);
await page.setViewportSize({ width: 1024, height: 800 });
await page.waitForTimeout(500);
check("playback controls fit the supported desktop width", await page.getByTestId("traffic-player").evaluate((el) => el.scrollWidth <= el.clientWidth));
await page.getByRole("button", { name: "Close traffic playback" }).click();
check("closing the player clears packets and retains the template's DNS diagram", await page.getByTestId("traffic-player").count() === 0 && await page.getByTestId("traffic-packet").count() === 0 && await page.locator('.react-flow__edge[data-id="traffic-display:dns-lookup"]').count() === 1);
await page.getByTestId("simulation-status").getByRole("button", { name: "Set capacity" }).click();
check("the capacity action opens the editable workload", (await page.locator('[data-mode="design"]').getAttribute("class")).includes("active") && await page.getByPlaceholder("e.g. 800").isVisible());
await page.getByPlaceholder("e.g. 800").fill("100");
check("editing the workload hides results for the previous inputs", await page.getByTestId("simulation-status").count() === 0);

await mode(page, "design");
await loadTemplate(page, "enterprise-aws-network");
await follow();
check("the enterprise template opens on the complete application journey", await page.getByLabel("Traffic journey", { exact: true }).inputValue() === "production_request");
await page.getByRole("button", { name: "Show hop 3" }).click();
check("the declared TLS application hop is replayable after DNS", (await page.getByTestId("traffic-progress").innerText()).includes("Hop 3 of 4") && await page.getByTestId("traffic-packet").count() === 1);
await page.getByRole("button", { name: "Show hop 4" }).click();
check("the complete request retains the unmodelled isolated database hop", (await page.getByTestId("traffic-progress").innerText()).includes("Stopped at hop 4 of 4") && (await page.getByTestId("traffic-player").innerText()).includes("route_selection") && await page.getByTestId("traffic-packet").count() === 0);
await close();
done("10-traffic-playback");
