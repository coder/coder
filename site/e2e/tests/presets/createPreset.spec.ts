import {expect, test} from "@playwright/test";
import {currentUser, login} from "../../helpers";
import {beforeCoderTest} from "../../hooks";
import path from "node:path";

test.beforeEach(async ({page}) => {
    beforeCoderTest(page);
    await login(page);
});

test("create template with preset and use in workspace", async ({page, baseURL}) => {
    test.setTimeout(120_000);

    // Create new template.
    await page.goto('/templates/new', {waitUntil: 'domcontentloaded'});
    await page.getByTestId('drop-zone').click();

    // Select the template file.
    const [fileChooser] = await Promise.all([
        page.waitForEvent('filechooser'),
        page.getByTestId('drop-zone').click()
    ]);
    await fileChooser.setFiles(path.join(__dirname, 'template.zip'));

    // Set name and submit.
    const templateName = generateRandomName();
    await page.locator("input[name=name]").fill(templateName);
    await page.getByRole('button', {name: 'Save'}).click();

    await page.waitForURL(`/templates/${templateName}/files`, {
        timeout: 120_000,
    });

    // Visit workspace creation page for new template.
    await page.goto(`/templates/default/${templateName}/workspace`, {waitUntil: 'domcontentloaded'});

    await page.locator('button[aria-label="Preset"]').click();

    const preset1 = page.getByText('I Like GoLand');
    const preset2 = page.getByText('Some Like PyCharm');

    await expect(preset1).toBeVisible();
    await expect(preset2).toBeVisible();

    // Choose the GoLand preset.
    await preset1.click();

    // Validate the preset was applied correctly.
    await expect(page.locator('input[value="GO"]')).toBeChecked();

    // Create a workspace.
    const workspaceName = generateRandomName();
    await page.locator("input[name=name]").fill(workspaceName);
    await page.getByRole('button', {name: 'Create workspace'}).click();

    // Wait for the workspace build display to be navigated to.
    const user = currentUser(page);
    await page.waitForURL(`/@${user.username}/${workspaceName}`, {
        timeout: 120_000, // Account for workspace build time.
    });

    // Visit workspace settings page.
    await page.goto(`/@${user.username}/${workspaceName}/settings/parameters`);

    // Validate the preset was applied correctly.
    await expect(page.locator('input[value="GO"]')).toBeChecked();
});

function generateRandomName() {
    const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ';
    let name = '';
    for (let i = 0; i < 10; i++) {
        name += chars.charAt(Math.floor(Math.random() * chars.length));
    }
    return name;
}
