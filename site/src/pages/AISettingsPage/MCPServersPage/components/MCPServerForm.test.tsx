import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { render } from "#/testHelpers/renderHelpers";
import { MockCoderMCPServer } from "../testFixtures";
import { MCPServerForm } from "./MCPServerForm";

it.each(["", "replacement-signing-secret"])(
	"saves only a nonempty replacement signing secret: %s",
	async (secret) => {
		const user = userEvent.setup();
		const onUpdateServer = vi.fn(async () => undefined);
		render(
			<MCPServerForm
				server={MockCoderMCPServer}
				listPath="/servers"
				isSaving={false}
				isDeleting={false}
				canSelectUserOIDC
				onUpdateServer={onUpdateServer}
				onCancel={vi.fn()}
			/>,
		);
		await user.type(screen.getByLabelText(/display name/i), " Updated");
		await user.click(screen.getByRole("button", { name: /behavior/i }));
		const input = screen.getByLabelText("Signing secret");
		await user.click(input);
		if (secret) await user.type(input, secret);
		await user.click(screen.getByRole("button", { name: "Update server" }));
		await waitFor(() => {
			expect(onUpdateServer).toHaveBeenCalledWith(
				MockCoderMCPServer.id,
				expect.objectContaining({ signing_secret: secret || undefined }),
			);
		});
	},
);

const renderCreateAPIKeyForm = async () => {
	const user = userEvent.setup();
	const onCreateServer = vi.fn(async () => undefined);
	render(
		<MCPServerForm
			isSaving={false}
			canSelectUserOIDC
			onCreateServer={onCreateServer}
		/>,
	);
	await user.type(screen.getByLabelText(/display name/i), "GitHub");
	await user.type(
		screen.getByLabelText(/server url/i),
		"https://api.githubcopilot.com/mcp/",
	);
	await user.click(screen.getByRole("button", { name: /authentication/i }));
	await user.click(
		screen.getByRole("combobox", { name: /authentication method/i }),
	);
	await user.click(screen.getByRole("option", { name: "API key" }));
	return {
		user,
		onCreateServer,
		addServer: screen.getByRole("button", { name: "Add server" }),
		headerName: screen.getByLabelText(/header name/i),
		headerValue: screen.getByLabelText(/header value/i),
	};
};

it("creates an API key server only once a header value is entered", async () => {
	const { user, onCreateServer, addServer, headerValue } =
		await renderCreateAPIKeyForm();

	await user.click(addServer);
	// Clearing a typed value and leaving the field restores the masked
	// placeholder even though no key is saved.
	await user.type(headerValue, "x");
	await user.clear(headerValue);
	await user.tab();
	await user.click(addServer);
	expect(onCreateServer).not.toHaveBeenCalled();

	await user.type(headerValue, "Bearer x");
	await user.click(addServer);
	await waitFor(() => {
		expect(onCreateServer).toHaveBeenCalledWith(
			expect.objectContaining({
				auth_type: "api_key",
				api_key_header: "Authorization",
				api_key_value: "Bearer x",
			}),
		);
	});
});

it("does not create an API key server without a header name", async () => {
	const { user, onCreateServer, addServer, headerName, headerValue } =
		await renderCreateAPIKeyForm();

	await user.type(headerValue, "Bearer x");
	await user.clear(headerName);
	await user.type(headerName, " ");
	await user.click(addServer);
	expect(onCreateServer).not.toHaveBeenCalled();
});

it("updates an API key server without re-entering the saved key", async () => {
	const user = userEvent.setup();
	const onUpdateServer = vi.fn(async () => undefined);
	const server = {
		...MockCoderMCPServer,
		auth_type: "api_key",
		api_key_header: "X-API-Key",
		has_api_key: true,
	};
	render(
		<MCPServerForm
			server={server}
			listPath="/servers"
			isSaving={false}
			isDeleting={false}
			canSelectUserOIDC
			onUpdateServer={onUpdateServer}
			onCancel={vi.fn()}
		/>,
	);
	await user.type(screen.getByLabelText(/display name/i), " Updated");
	await user.click(screen.getByRole("button", { name: "Update server" }));
	await waitFor(() => {
		expect(onUpdateServer).toHaveBeenCalledWith(
			server.id,
			expect.objectContaining({
				api_key_header: "X-API-Key",
				api_key_value: undefined,
			}),
		);
	});
});
