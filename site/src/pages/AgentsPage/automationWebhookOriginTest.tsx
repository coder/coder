import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { expect } from "vitest";
import { MockBuildInfo, MockDefaultOrganization } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import AgentAutomationEditorPage from "./AgentAutomationEditorPage";

/**
 * Opens a new webhook automation with the given access URL and checks the
 * endpoint it shows. The test file sets the page origin with
 * @vitest-environment-options, so the URL there is what this tab uses.
 */
export const expectNewWebhookEndpoint = async (
	accessURL: string,
	expectedOrigin: string,
) => {
	const user = userEvent.setup();
	server.use(
		http.get("/api/v2/buildinfo", () =>
			HttpResponse.json({ ...MockBuildInfo, dashboard_url: accessURL }),
		),
		http.get("/api/v2/experiments", () =>
			HttpResponse.json(["chat-automations"]),
		),
		http.get("/api/v2/organizations", () =>
			HttpResponse.json([MockDefaultOrganization]),
		),
		http.get("/api/v2/chats", () => HttpResponse.json([])),
		http.get("/api/v2/organizations/:organizationId/chats/models", () =>
			HttpResponse.json({
				models: [],
				providers: [],
				unsupported_providers: [],
			}),
		),
	);
	renderWithAuth(<AgentAutomationEditorPage />, {
		path: "/agents/automations/new",
		route: "/agents/automations/new",
	});

	await user.click(await screen.findByRole("radio", { name: "Webhook" }));

	const form = screen.getByRole("form", { name: "New automation" });
	expect(window.location.origin).not.toBe(new URL(accessURL).origin);
	within(form).getByText(
		`${expectedOrigin}/api/experimental/chat-automations/<automation-id>/events`,
	);
};
