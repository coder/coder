import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { expect, it } from "vitest";
import { MockACPSession } from "#/testHelpers/acp";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { ACPTool, acpChatPath } from "./ACPTool";
import { Tool } from "./Tool";

it("opens the custom ACP chat from the session card", async () => {
	const session = MockACPSession;
	const destination = acpChatPath(
		session.parent_chat_id,
		session.workspace_agent_id,
		session.session_id,
	);
	const router = createMemoryRouter([
		{
			path: "/",
			element: (
				<ACPTool
					ToolComponent={Tool}
					name="acp_spawn_agent"
					status="completed"
					args={{}}
					result={session}
					isError={false}
				/>
			),
		},
		{ path: destination, element: <div /> },
	]);
	renderWithRouter(router);
	await userEvent.click(
		screen.getByRole("link", { name: `Open ${session.title}` }),
	);
	expect(router.state.location.pathname).toBe(destination);
});
