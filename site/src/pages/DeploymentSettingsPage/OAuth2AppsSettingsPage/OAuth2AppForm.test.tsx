import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockOAuth2ProviderApps } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { OAuth2AppForm } from "./OAuth2AppForm";

describe("OAuth2AppForm", () => {
	it("submits dynamically registered client values", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();

		render(
			<OAuth2AppForm onSubmit={onSubmit} isUpdating={false} disabled={false} />,
		);

		await user.type(screen.getByLabelText(/^name/i), "VS Code Coder Extension");
		await user.type(
			screen.getByLabelText(/callback url/i),
			"vscode://coder.coder-remote/oauth/callback",
		);
		await user.click(
			screen.getByRole("button", { name: /create application/i }),
		);

		await waitFor(() => {
			expect(onSubmit).toHaveBeenCalledWith({
				name: "VS Code Coder Extension",
				callback_url: "vscode://coder.coder-remote/oauth/callback",
				icon: "",
			});
		});
	});

	it("submits edited dynamically registered client values", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const app = {
			...MockOAuth2ProviderApps[0],
			name: "VS Code Coder Extension",
			callback_url: "vscode://coder.coder-remote/oauth/callback",
		};

		render(
			<OAuth2AppForm
				app={app}
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
		);

		await user.clear(screen.getByLabelText(/^name/i));
		await user.type(screen.getByLabelText(/^name/i), "Cursor MCP Extension");
		await user.click(
			screen.getByRole("button", { name: /update application/i }),
		);

		await waitFor(() => {
			expect(onSubmit).toHaveBeenCalledWith({
				name: "Cursor MCP Extension",
				callback_url: "vscode://coder.coder-remote/oauth/callback",
				icon: app.icon,
			});
		});
	});

	it("does not submit a dangerous callback URL", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();

		render(
			<OAuth2AppForm onSubmit={onSubmit} isUpdating={false} disabled={false} />,
		);

		await user.type(screen.getByLabelText(/^name/i), "test-app");
		const callbackURL = screen.getByLabelText(/callback url/i);
		// oxlint-disable-next-line eslint/no-script-url -- Deliberately invalid input exercises callback URL rejection.
		await user.type(callbackURL, "javascript:alert(1)");
		await user.click(
			screen.getByRole("button", { name: /create application/i }),
		);

		expect(onSubmit).not.toHaveBeenCalled();
	});
});
