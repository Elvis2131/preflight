import { open, check, done, loadTemplate, mode, baseline } from "./lib.mjs";

const { page, close } = await open();

await page.evaluate(() => localStorage.removeItem("preflight-theme"));
await page.emulateMedia({ colorScheme: "dark" });
await page.reload();
await page.getByRole("button", { name: "Switch to light mode" }).waitFor();
check("first visit respects the system dark theme", await page.locator("html").getAttribute("data-theme") === "dark");
await page.getByRole("button", { name: "Switch to light mode" }).click();
await page.getByRole("button", { name: "Switch to dark mode" }).waitFor();
await page.reload();
check("explicit light choice survives reload and overrides system dark", await page.locator("html").getAttribute("data-theme") === "light");
await page.getByRole("button", { name: "Switch to dark mode" }).click();
await page.getByRole("button", { name: "Switch to light mode" }).waitFor();
await page.reload();
check("explicit dark choice survives reload", await page.locator("html").getAttribute("data-theme") === "dark");

// Simulate an outdated server once, then recover against the actual assessd endpoint.
await page.route("**/templates", (route) => route.fulfill({ status: 404, body: "404 page not found" }));
await page.reload();
await page.getByRole("button", { name: "Retry templates" }).waitFor();
check("template failures explain the backend mismatch", (await page.getByRole("alert").innerText()).includes("does not support templates"));
check("unavailable templates cannot be selected", await page.getByTestId("template-picker").isDisabled());
await page.unroute("**/templates");
await page.getByRole("button", { name: "Retry templates" }).click();
await page.waitForFunction(() => document.querySelector('[data-testid="template-picker"]')?.options.length > 1);
check("retry restores real templates without reloading the page", !await page.getByTestId("template-picker").isDisabled());
await loadTemplate(page, "three-tier-vpc");
check("recovered picker loads actual architecture services", await page.locator(".react-flow__node-golden").count() === 5);
await page.locator('.react-flow__node[data-id="aws_eks_cluster.app"]').dispatchEvent("click");
await page.locator(".inspector-panel").waitFor();

const textContrast = async (selector) => page.locator(selector).evaluateAll((elements) => {
  const rgb = (value) => value.match(/[\d.]+/g)?.slice(0, 3).map(Number);
  const luminance = (channels) => channels.map((c) => {
    const x = c / 255;
    return x <= 0.04045 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4;
  }).reduce((sum, value, i) => sum + value * [0.2126, 0.7152, 0.0722][i], 0);
  return elements.filter((el) => el.getBoundingClientRect().height > 0).flatMap((el) => {
    let background = el;
    while (background.parentElement && getComputedStyle(background).backgroundColor === "rgba(0, 0, 0, 0)") background = background.parentElement;
    const foreground = luminance(rgb(getComputedStyle(el).color));
    const behind = luminance(rgb(getComputedStyle(background).backgroundColor));
    const ratio = (Math.max(foreground, behind) + 0.05) / (Math.min(foreground, behind) + 0.05);
    return ratio < 4.5 ? [`${el.className}: ${ratio.toFixed(2)}`] : [];
  });
});
const contrastFailures = await textContrast(".library-title h2, .canvas-title, .service-node-body, .inspector-heading h2, .setting-help");
check("dark canvas and settings text meet 4.5:1 contrast", contrastFailures.length === 0, contrastFailures.join(", "));
await page.locator(".inspector-panel").getByRole("button", { name: "Close service settings" }).click();
await page.getByRole("toolbar").getByRole("button", { name: "Workload", exact: true }).click();
const workloadName = page.locator(".workload-panel").locator("xpath=.//label[text()='name']/following-sibling::input[1]");
await workloadName.fill("theme-persistence-check");
const workloadContrast = await textContrast(".workload-panel label");
check("dark workload labels including compliance options are readable", workloadContrast.length === 0, workloadContrast.join(", "));
await page.getByRole("button", { name: "Switch to light mode" }).click();
check("theme changes retain edited workload inputs", await workloadName.inputValue() === "theme-persistence-check");
await page.getByRole("button", { name: "Switch to dark mode" }).click();
await mode(page, "simulate");
await baseline(page);
check("dark mode preserves real baseline simulations", await page.getByTestId("simulation-status").count() === 1);
await page.getByRole("button", { name: "Switch to light mode" }).click();
check("switching theme preserves simulation results", await page.getByTestId("simulation-status").count() === 1);

await close();
done("14-theme-and-template-recovery");
