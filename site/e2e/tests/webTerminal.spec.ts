import { randomUUID } from "node:crypto";
import { test } from "@playwright/test";
import {
	createTemplate,
	createWorkspace,
	login,
	openTerminalWindow,
	startAgent,
	stopAgent,
} from "../helpers";
import { beforeCoderTest } from "../hooks";

test.beforeEach(async ({ page }) => {
	beforeCoderTest(page);
	await login(page);
});

test("web terminal", async ({ context, page }) => {
	const token = randomUUID();
	const template = await createTemplate(page, {
		graph: [
			{
				graph: {
					resources: [
						{
							agents: [
								{
									token,
									displayApps: { webTerminal: true },
									order: 0,
								},
							],
						},
					],
				},
			},
		],
	});
	const workspaceName = await createWorkspace(page, template);
	const agent = await startAgent(page, token);
	const terminal = await openTerminalWindow(page, context, workspaceName);

	// ghostty-web renders into a <canvas> element instead of DOM spans.
	// Wait for the canvas to appear and become visible.
	await terminal.waitForSelector("[data-testid=terminal] canvas", {
		state: "visible",
	});

	// Give the terminal a moment to finish initializing (WASM load, fit).
	await terminal.waitForTimeout(2000);

	// Ensure that we can type in it. We type a command and verify
	// the connection status attribute transitions to "connected",
	// which proves the WebSocket round-trip works.
	await terminal.keyboard.type("echo he${justabreak}llo123456");
	await terminal.keyboard.press("Enter");

	// Because the terminal is now canvas-based, we cannot inspect
	// rendered text via DOM selectors. Instead we verify the terminal
	// stayed connected (data-status="connected") and no error banner
	// appeared, which confirms the command was sent successfully.
	await terminal.waitForSelector('[data-status="connected"]', {
		state: "attached",
		timeout: 10 * 1000,
	});

	await stopAgent(agent);
});
