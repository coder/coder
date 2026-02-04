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

test("chats page has input textarea ready for immediate use", async ({
	page,
}) => {
	await page.goto("/chats");

	// Should see a textarea/input ready to type - no dialogs needed
	const chatInput = page.getByPlaceholder(/message/i);
	await expect(chatInput).toBeVisible();
	await expect(chatInput).toBeFocused();
});

test("typing a message and pressing enter creates chat and sends message", async ({
	page,
}) => {
	// This test requires a real LLM API key
	test.skip(!hasAnthropicKey, "ANTHROPIC_API_KEY not set");
	test.setTimeout(180000);

	await page.goto("/chats");

	// Type directly into the chat input
	const chatInput = page.getByPlaceholder(/message/i);
	await expect(chatInput).toBeVisible();
	await chatInput.fill("Say hello");
	await chatInput.press("Enter");

	// Should navigate to a chat detail page
	await expect(page).toHaveURL(/\/chats\/[a-f0-9-]+$/, { timeout: 10000 });

	// Should see the message and eventually an assistant response
	await expect(page.getByText("You").first()).toBeVisible({ timeout: 10000 });
	await expect(page.getByText("Assistant").first()).toBeVisible({
		timeout: 120000,
	});
});

test("recent chats are shown below the input", async ({ page }) => {
	// This test requires a real LLM API key to create a chat
	test.skip(!hasAnthropicKey, "ANTHROPIC_API_KEY not set");
	test.setTimeout(180000);

	await page.goto("/chats");

	// Create a chat by typing a message
	const chatInput = page.getByPlaceholder(/message/i);
	const uniqueMessage = `Test message ${Date.now()}`;
	await chatInput.fill(uniqueMessage);
	await chatInput.press("Enter");

	// Wait for chat to be created and navigate
	await expect(page).toHaveURL(/\/chats\/[a-f0-9-]+$/, { timeout: 10000 });

	// Go back to chats list
	await page.goto("/chats");

	// Should see recent chats section below the input
	await expect(page.getByText("Recent Chats")).toBeVisible();

	// The chat should appear in the recent list - look for links that go to /chats/uuid
	await expect(
		page.getByRole("link", { name: /Untitled Chat/i }).first(),
	).toBeVisible();
});

test("clicking a recent chat navigates to chat detail", async ({ page }) => {
	// This test requires a real LLM API key to create a chat
	test.skip(!hasAnthropicKey, "ANTHROPIC_API_KEY not set");
	test.setTimeout(180000);

	await page.goto("/chats");

	// Create a chat first
	const chatInput = page.getByPlaceholder(/message/i);
	await chatInput.fill("Hello for navigation test");
	await chatInput.press("Enter");

	// Wait for chat creation
	await expect(page).toHaveURL(/\/chats\/[a-f0-9-]+$/, { timeout: 10000 });

	// Go back to chats list
	await page.goto("/chats");

	// Click on a recent chat (look for links with "Untitled Chat" text)
	const recentChatLink = page
		.getByRole("link", { name: /Untitled Chat/i })
		.first();
	await recentChatLink.click();

	// Should be on a chat detail page
	await expect(page).toHaveURL(/\/chats\/[a-f0-9-]+$/);
});
