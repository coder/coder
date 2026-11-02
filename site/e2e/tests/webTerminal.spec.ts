import { randomUUID } from "node:crypto";
import { expect, test } from "@playwright/test";
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

	// The terminal renders to a canvas, so read its output from the PTY
	// WebSocket instead of the DOM.
	let output = "";
	context.on("page", (newPage) => {
		newPage.on("websocket", (websocket) => {
			if (!websocket.url().includes("/pty")) {
				return;
			}
			websocket.on("framereceived", ({ payload }) => {
				output += payload.toString();
			});
		});
	});
	const terminal = await openTerminalWindow(page, context, workspaceName);

	await terminal.waitForSelector('[data-status="connected"]', {
		state: "visible",
		timeout: 30_000,
	});

	// Click the terminal to ensure it has keyboard focus after the
	// confirmation dialog.
	const terminalCanvas = terminal.getByTestId("terminal").locator("canvas");
	await terminalCanvas.waitFor({ state: "visible" });
	await terminalCanvas.click();

	// Ensure that we can type in it
	await terminal.keyboard.type("echo he${justabreak}llo123456");
	await terminal.keyboard.press("Enter");

	// Check if "echo" command was executed. The typed command contains
	// "${justabreak}", so only the command's output matches.
	await expect
		.poll(() => output, { timeout: 10 * 1000 })
		.toContain("hello123456");

	await stopAgent(agent);
});
