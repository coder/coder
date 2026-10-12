/**
 * @vitest-environment-options {"url": "http://127.0.0.1:13491/agents/automations/new"}
 */
import { describe, it, vi } from "vitest";
import { expectNewWebhookEndpoint } from "./automationWebhookOriginTest";

// AgentPageHeader needs the Agents layout's outlet context.
vi.mock("./components/AgentPageHeader", () => ({
	AgentPageHeader: () => null,
}));

describe("New webhook endpoint through a port forward", () => {
	it("shows the forwarded port, not the loopback access URL", async () => {
		await expectNewWebhookEndpoint(
			"http://127.0.0.1:3100",
			"http://127.0.0.1:13491",
		);
	});
});
