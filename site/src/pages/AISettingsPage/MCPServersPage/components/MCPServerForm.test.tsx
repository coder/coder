import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { render, renderWithRouter } from "#/testHelpers/renderHelpers";
import { MockCoderMCPServer } from "../testFixtures";
import { MCPServerForm } from "./MCPServerForm";

it("regenerates the signing secret only after confirmation", async () => {
	const user = userEvent.setup();
	const onRegenerateSigningSecret = vi.fn();
	render(
		<MCPServerForm
			server={MockCoderMCPServer}
			listPath="/servers"
			isSaving={false}
			isDeleting={false}
			isRegeneratingSigningSecret={false}
			canSelectUserOIDC
			onRegenerateSigningSecret={onRegenerateSigningSecret}
			onCancel={vi.fn()}
		/>,
	);

	await user.click(
		screen.getByRole("button", { name: "Regenerate signing secret" }),
	);
	expect(onRegenerateSigningSecret).not.toHaveBeenCalled();
	await user.click(screen.getByRole("button", { name: "Cancel" }));
	expect(onRegenerateSigningSecret).not.toHaveBeenCalled();
	await user.click(
		screen.getByRole("button", { name: "Regenerate signing secret" }),
	);
	await user.click(screen.getByRole("button", { name: "Regenerate" }));
	expect(onRegenerateSigningSecret).toHaveBeenCalledOnce();
});

it("clears unsaved changes before navigating after creation", async () => {
	const user = userEvent.setup();
	const onCreateServer = vi.fn(async () => ({
		afterSave: () => void router.navigate("/servers"),
	}));
	const router = createMemoryRouter([
		{
			path: "/",
			element: (
				<MCPServerForm
					listPath="/servers"
					isSaving={false}
					canSelectUserOIDC
					onCreateServer={onCreateServer}
				/>
			),
		},
		{ path: "/servers", element: <div>Servers</div> },
	]);
	renderWithRouter(router);
	await user.type(screen.getByLabelText(/display name/i), "GitHub");
	await user.type(
		screen.getByLabelText(/server url/i),
		"https://api.githubcopilot.com/mcp/",
	);
	await user.click(screen.getByRole("button", { name: "Add server" }));
	await waitFor(() => {
		expect(router.state.location.pathname).toBe("/servers");
	});
	expect(onCreateServer).toHaveBeenCalledOnce();
});
