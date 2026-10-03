// PC-131: build a multi-fault scenario, run it, save it, then CHANGE the design and re-run:
// the same saved definition is re-evaluated against the current design (not replayed), and the
// report lists it with that current result.
import { open, check, done, mode, select, loadTemplate, sessionLabel } from "./lib.mjs";

const { page, close } = await open();
await loadTemplate(page, "event-driven");
await page.getByRole("toolbar", { name: "Architecture tools" }).getByRole("button", { name: "Workload", exact: true }).click();
const jf = (ph) => page.locator(`xpath=//input[@placeholder="${ph}" and not(ancestor::aside)]`).last();
await page.click("text=+ journey");
await jf("id").fill("ingest");
await jf("name").fill("Ingest");
await jf("path (e.g. internet, lb-1, db-1)").fill("internet, aws_route53_record.ingest, aws_lambda_function.producer");
await jf("protocol").fill("tcp");
await jf("port").fill("443");
await jf("criticality").fill("tier2");

await mode(page, "failure_lab");
const lab = page.locator('[data-testid="failure-lab"]');
await lab.locator('[data-testid="fault-kind"]').selectOption("node_loss");
for (const t of ["aws_lambda_function.producer", "aws_lambda_function.consumer"]) {
  await lab.locator('[data-testid="fault-target"]').selectOption(t);
  await lab.locator('[data-testid="add-fault"]').click();
}
check("a two-fault scenario is built", (await lab.locator('[data-testid="scenario-faults"] li').count()) === 2);
await lab.locator('[data-testid="fault-kind"]').selectOption("external_dependency_outage");
check("a kind with nothing to apply to offers no target", (await lab.locator('[data-testid="fault-target"]').count()) === 0);
await lab.locator('[data-testid="fault-kind"]').selectOption("node_loss");

await lab.locator('[data-testid="run-scenario"]').click();
await page.waitForTimeout(2200);
check("running it assesses the design (a version appears)", /latest v\d/.test(await sessionLabel(page)));
check("the server's structural result is shown in plain language", (await page.getByTestId("simulation-status").innerText()).includes("Disconnected destinations:") && (await page.getByTestId("simulation-status").innerText()).includes("Fault simulation"));

await lab.locator('[data-testid="scenario-name"]').fill("lose both lambdas");
await lab.locator('[data-testid="save-scenario"]').click();
await page.waitForTimeout(600);
check("the scenario is saved", (await lab.locator('[data-testid="saved-scenarios"] li').count()) === 1);

await lab.locator('[data-testid="rerun-all"]').click();
await page.waitForTimeout(2200);
const first = await lab.locator('[data-testid="scenario-results"]').innerText();
check("re-run against the unchanged design is assessed", /verdict:/.test(first) && !/not assessable/.test(first), first.slice(0, 160));

await mode(page, "design");
await select(page, "aws_lambda_function.consumer");
await page.keyboard.press("Backspace"); // remove the consumer from the design
await page.waitForTimeout(400);
await mode(page, "failure_lab");
await lab.locator('[data-testid="rerun-all"]').click();
await page.waitForTimeout(2200);
const second = await lab.locator('[data-testid="scenario-results"]').innerText();
check("after the design changed, the SAME saved scenario is re-evaluated: not assessable, naming the missing node",
  /not assessable/.test(second) && /aws_lambda_function\.consumer/.test(second), second.slice(0, 200));

await mode(page, "report");
await page.click("text=Generate / view report");
await page.waitForTimeout(2000);
const card = await page.locator('[data-testid="report-scenarios"]').innerText().catch(() => "");
check("the report lists the scenario with its CURRENT result", /lose both lambdas/.test(card) && /not assessable/.test(card), card.slice(0, 160));
await close();
done("03-failure-lab");
