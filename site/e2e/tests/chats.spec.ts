import { expect, test } from "@playwright/test";
import { login } from "../helpers";
import { beforeCoderTest } from "../hooks";

test.describe.configure({ mode: "parallel" });

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
});

// Skip LLM-dependent tests if ANTHROPIC_API_KEY is not set
const hasAnthropicKey = Boolean(process.env.ANTHROPIC_API_KEY);

test("create a chat and navigate to chat detail", async ({ page }) => {
	// Navigate to chats page
	await page.goto("/chats");
	await expect(page.getByRole("heading", { name: "Chats" })).toBeVisible();

	// Click create chat button
	await page.getByRole("button", { name: "New Chat" }).click();

	// Fill in the dialog - use claude-haiku-4-5 for faster/cheaper e2e tests
	await expect(page.getByRole("dialog")).toBeVisible();
	await page.getByLabel("Title").fill("Test Chat E2E");
	await page.getByLabel("Model").fill("claude-haiku-4-5");
	await page.getByRole("button", { name: "Create" }).click();

	// Should navigate to the chat detail page
	await expect(page).toHaveURL(/\/chats\/[a-f0-9-]+$/);

	// Should see the chat title
	await expect(
		page.getByRole("heading", { name: "Test Chat E2E" }),
	).toBeVisible();

	// Should see the provider/model info
	await expect(page.getByText("anthropic")).toBeVisible();
});

test("send a message and receive a response", async ({ page }) => {
	// This test requires a real LLM API key
	test.skip(!hasAnthropicKey, "ANTHROPIC_API_KEY not set");

	// Increase timeout for this test since LLM responses can take a while
	test.setTimeout(180000);

	// Navigate to chats page
	await page.goto("/chats");
	await expect(page.getByRole("heading", { name: "Chats" })).toBeVisible();

	// Create a new chat - use claude-haiku-4-5 for faster/cheaper e2e tests
	await page.getByRole("button", { name: "New Chat" }).click();
	await expect(page.getByRole("dialog")).toBeVisible();
	await page.getByLabel("Title").fill("Message Test Chat");
	await page.getByLabel("Model").fill("claude-haiku-4-5");
	await page.getByRole("button", { name: "Create" }).click();

	// Wait for navigation to chat detail
	await expect(page).toHaveURL(/\/chats\/[a-f0-9-]+$/);

	// Send a simple message
	const messageInput = page.getByPlaceholder(/Type your message/);
	await expect(messageInput).toBeVisible();
	await messageInput.fill("Hello, please respond with just the word 'Hello'");

	// Click send button
	await page.getByRole("button", { name: "Send" }).click();

	// Wait for the user message to appear
	await expect(page.getByText("You").first()).toBeVisible({ timeout: 10000 });

	// Wait for an assistant response - be liberal with expectations since it's a real LLM
	// We just check that an "Assistant" label appears, indicating a response was received.
	// Give it a long timeout because LLM responses can take time.
	await expect(page.getByText("Assistant").first()).toBeVisible({
		timeout: 120000,
	});
});

test("chat list shows created chats", async ({ page }) => {
	// Navigate to chats page
	await page.goto("/chats");
	await expect(page.getByRole("heading", { name: "Chats" })).toBeVisible();

	// Create a chat with a unique name - use claude-haiku-4-5 for faster/cheaper e2e tests
	const chatTitle = `List Test ${Date.now()}`;
	await page.getByRole("button", { name: "New Chat" }).click();
	await expect(page.getByRole("dialog")).toBeVisible();
	await page.getByLabel("Title").fill(chatTitle);
	await page.getByLabel("Model").fill("claude-haiku-4-5");
	await page.getByRole("button", { name: "Create" }).click();

	// Wait for navigation to chat detail
	await expect(page).toHaveURL(/\/chats\/[a-f0-9-]+$/);

	// Navigate back to chats list
	await page.getByRole("link", { name: "Back to Chats" }).click();
	await expect(page).toHaveURL("/chats");

	// The chat should appear in the list
	await expect(page.getByRole("link", { name: chatTitle })).toBeVisible();
});
