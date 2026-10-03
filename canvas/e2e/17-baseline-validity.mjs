// PC-161: the result says whether traffic flowed BEFORE the fault, separately from what the fault did.
// A working design: journeys flow before and one is broken by the fault. Then the design is made to never
// carry one journey (its private route removed): the result names it as already blocked, withholds the
// survival verdict, and the Failure Lab does not list it as failed by the scenario.
import { open, check, done, mode, select, loadTemplate } from "./lib.mjs";

const { page, close } = await open();
await loadTemplate(page, "three-tier-vpc");
await mode(page, "failure_lab");
const lab = page.locator('[data-testid="failure-lab"]');
const loseDatabase = async () => {
  await lab.locator('[data-testid="fault-kind"]').selectOption("node_loss");
  await lab.locator('[data-testid="fault-target"]').selectOption("aws_db_instance.db");
  await lab.locator('[data-testid="add-fault"]').click();
  await lab.locator('[data-testid="run-scenario"]').click();
  await page.waitForTimeout(2500);
};
const status = (id) => page.locator(`[data-testid="baseline-journey-${id}"]`).getAttribute("data-status");

await loseDatabase();
const summary = page.getByTestId("baseline-summary");
check("the baseline is the first thing in the result", await summary.count() === 1 && (await page.getByTestId("simulation-status").innerText()).startsWith("Does traffic flow before the fault?"));
check("a working design: traffic flows before the fault", (await summary.getAttribute("data-state")) === "assessed" && (await summary.innerText()).includes("Traffic flows through every declared journey before the fault."));
check("web flowed before and after", (await status("web")) === "flows_before_and_after");
check("data flowed before and the fault breaks it", (await status("data")) === "broken_by_fault");

// A real deny: declare the api journey's last hop on a port its security group does not admit, so it
// never carried traffic.
const setApiHopPort = async (value) => {
  await mode(page, "design");
  if (await page.locator(".journey-hop-ports").count() === 0) await page.locator('button:has-text("Workload")').first().click();
  const api = page.locator(".journey-hop-ports").nth(1);
  await api.locator("summary").click();
  const inputs = api.locator('input[type="number"]');
  await inputs.nth((await inputs.count()) - 1).fill(value);
  await mode(page, "failure_lab");
};
const rerun = async () => {
  if (await lab.locator('[data-testid="scenario-faults"] li').count() === 0) await loseDatabase();
  else { await lab.locator('[data-testid="run-scenario"]').click(); await page.waitForTimeout(2500); }
};
await setApiHopPort("2222");
await rerun();
check("api never carried traffic: already blocked, not a result of the fault", (await status("api")) === "already_blocked");
check("the summary says some journeys never carried traffic", (await summary.getAttribute("data-tone")) === "warn" && (await summary.innerText()).includes("never carried traffic"));
const text = await page.getByTestId("simulation-status").innerText();
check("the survival verdict is withheld, not 'no impact'", text.includes("No survival verdict") && !text.includes("No structural impact detected"));

await lab.locator('[data-testid="scenario-name"]').fill("lose the database");
await lab.locator('[data-testid="save-scenario"]').click();
await page.waitForTimeout(600);
await lab.locator('[data-testid="rerun-all"]').click();
await page.waitForTimeout(2500);
const results = await lab.locator('[data-testid="scenario-results"]').innerText();
check("the Failure Lab lists api as already blocked", (await lab.locator('[data-testid="scenario-already-blocked"]').innerText()).includes("api"));
check("and does not list api as broken by the fault", !/broken by the fault:[^\n]*\bapi\b/.test(results), results.slice(0, 300));

// A hop the engine cannot DECIDE is "could not be checked", never "already blocked": remove the private route
// table's only route (the subnet is left with no resolvable effective route table).
await setApiHopPort("");
await mode(page, "design");
await select(page, "aws_eks_cluster.app");
await page.getByTestId("configuration-subnets").getByRole("button", { name: "Edit Private A", exact: true }).click();
await page.getByTestId("configuration-route_table").getByRole("button", { name: "Edit Private route table", exact: true }).click();
await page.locator('[data-testid="routes-editor"] button:has-text("×")').first().click();
await mode(page, "failure_lab");
await rerun();
check("an undecidable route is 'could not be checked', not 'already blocked'", (await status("api")) === "not_assessable");
check("and the summary says the baseline was not fully checked", (await summary.getAttribute("data-state")) === "not_assessable" && (await summary.innerText()).includes("not fully checked"));

await close();
done("17-baseline-validity");
