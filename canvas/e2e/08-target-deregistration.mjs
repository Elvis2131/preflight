// PC-130: the Failure Lab offers target deregistration for a load balancer whose targets are
// drawn on the canvas, names exactly the target to drop, and shows the SERVER's answer: the
// journey that crossed that LB -> target hop fails; the one that did not is untouched.
import { open, check, done, mode, loadTemplate } from "./lib.mjs";

const { page, close } = await open();
await loadTemplate(page, "three-tier-vpc");
await mode(page, "failure_lab");
const lab = page.locator('[data-testid="failure-lab"]');

await lab.locator('[data-testid="fault-kind"]').selectOption("target_deregistration");
const lbs = await lab.locator('[data-testid="fault-target"] option').allInnerTexts();
check("only the load balancer that has drawn targets is offered", lbs.length === 1 && /aws_lb\.web/.test(lbs[0]), lbs.join(" | "));
const targets = await lab.locator('[data-testid="fault-deregister-target"] option').allInnerTexts();
check("the registered targets of that load balancer are offered, and only those", targets.length === 1 && /aws_eks_cluster\.app/.test(targets[0]), targets.join(" | "));

await lab.locator('[data-testid="add-fault"]').click();
const listed = await lab.locator('[data-testid="scenario-faults"] li').allInnerTexts();
check("the declared fault is described in words", listed.length === 1 && /deregister aws_eks_cluster\.app from aws_lb\.web/.test(listed[0]), listed.join(" | "));

await lab.locator('[data-testid="run-scenario"]').click();
await page.waitForTimeout(2500);
await lab.locator('[data-testid="scenario-name"]').fill("deregister the app from the LB");
await lab.locator('[data-testid="save-scenario"]').click();
await page.waitForTimeout(600);
await lab.locator('[data-testid="rerun-all"]').click();
await page.waitForTimeout(2500);
const result = await lab.locator('[data-testid="scenario-results"]').innerText();
const failed = (result.match(/failed journeys:[^\n]*/) ?? ["(none)"])[0];
check("the server reports the LB -> app journey (api) as failed", /\bapi\b/.test(failed), failed);
check("the journeys that do not cross that hop are not reported failed", !/\b(web|data)\b/.test(failed), failed);
await close();
done("08-target-deregistration");
