import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { Tool } from "./Tool";

// The View agent destination is the one part of the subagent card Pixel
// cannot observe: the target travels through getSubagentChatId
// (result-over-args precedence) and safeBuildAgentChatPath before reaching
// the Link, so the router location is asserted after a real click.
const renderTool = ({
	name,
	args,
	result,
}: {
	name: string;
	args: unknown;
	result?: unknown;
}) =>
	renderWithRouter(
		createMemoryRouter(
			[
				{
					path: "/agents",
					element: (
						<Tool
							organizationId="test-org"
							name={name}
							status="running"
							args={args}
							result={result}
						/>
					),
				},
				{ path: "*", element: <div /> },
			],
			{ initialEntries: ["/agents"] },
		),
	);

describe("SubagentTool", () => {
	it("links to the sub-agent chat from the result record", async () => {
		const user = userEvent.setup();

		const { router } = renderTool({
			name: "spawn_agent",
			args: { title: "Workspace diagnostics" },
			result: {
				chat_id: "child-chat-id",
				title: "Workspace diagnostics",
				status: "pending",
			},
		});

		await user.click(screen.getByRole("link", { name: "View agent" }));

		expect(router.state.location.pathname).toBe("/agents/child-chat-id");
	});
});
