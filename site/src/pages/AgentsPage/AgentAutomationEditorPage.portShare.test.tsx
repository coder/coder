/**
 * @vitest-environment-options {"url": "https://3000--dev--eph--apps.dogfood.cdr.dev/agents/automations/new"}
 */
import { describe, it, vi } from "vitest";
import { expectNewWebhookEndpoint } from "./automationWebhookOriginTest";

// AgentPageHeader needs the Agents layout's outlet context.
vi.mock("./components/AgentPageHeader", () => ({
	AgentPageHeader: () => null,
}));

describe("New webhook endpoint behind a port share", () => {
	it("shows the page origin, not the loopback access URL", async () => {
		await expectNewWebhookEndpoint(
			"http://127.0.0.1:3100",
			"https://3000--dev--eph--apps.dogfood.cdr.dev",
		);
	});
});
