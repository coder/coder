import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockOAuth2ProviderApps } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { OAuth2AppForm } from "./OAuth2AppForm";

describe("OAuth2AppForm", () => {
	it.each([
		"VS Code Coder Extension",
		" VS Code Coder Extension",
		"VS Code Coder Extension ",
		" VS Code Coder Extension ",
	])("submits a trimmed name for %j", async (name) => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();

		render(
			<OAuth2AppForm onSubmit={onSubmit} isUpdating={false} disabled={false} />,
		);

		await user.type(screen.getByLabelText(/^name/i), name);
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
		await user.type(screen.getByLabelText(/^name/i), " Cursor MCP Extension ");
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

	it.each([
		// oxlint-disable-next-line eslint/no-script-url -- Deliberately invalid input exercises callback URL rejection.
		"javascript:alert(1)",
		// oxlint-disable-next-line eslint/no-script-url -- Scheme rejection is case insensitive.
		"JaVaScRiPt:alert(1)",
		"data:text/plain,hello",
		"file:///tmp/callback",
		"ftp://example.com/callback",
		"urn:example:callback",
		"http:foo",
		"https:/example.com",
		"http:///example.com",
	])("does not submit invalid callback %j", async (callback) => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();

		render(
			<OAuth2AppForm onSubmit={onSubmit} isUpdating={false} disabled={false} />,
		);

		await user.type(screen.getByLabelText(/^name/i), "test-app");
		const callbackURL = screen.getByLabelText(/callback url/i);
		await user.type(callbackURL, callback);
		await user.click(
			screen.getByRole("button", { name: /create application/i }),
		);

		await act(async () => {});
		expect(onSubmit).not.toHaveBeenCalled();
	});

	it.each([
		" vscode://coder.coder-remote/oauth/callback",
		"vscode://coder.coder-remote/oauth/callback ",
		" vscode://coder.coder-remote/oauth/callback ",
		"URN:ietf:wg:oauth:2.0:oob",
		"localhost:3000",
	])("submits trimmed callback %j", async (callback) => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		render(
			<OAuth2AppForm onSubmit={onSubmit} isUpdating={false} disabled={false} />,
		);
		await user.type(screen.getByLabelText(/^name/i), "OAuth App");
		await user.type(screen.getByLabelText(/callback url/i), callback);
		await user.click(
			screen.getByRole("button", { name: /create application/i }),
		);
		await waitFor(() =>
			expect(onSubmit).toHaveBeenCalledWith({
				name: "OAuth App",
				callback_url: callback.trim(),
				icon: "",
			}),
		);
	});

	it.each([
		{ name: "é".repeat(32), valid: true },
		{ name: "a" + "é".repeat(32), valid: false },
		{ name: "é".repeat(33), valid: false },
		{ name: "界".repeat(21), valid: true },
		{ name: "界".repeat(22), valid: false },
	])("enforces the UTF-8 byte limit for $name", async ({ name, valid }) => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		render(
			<OAuth2AppForm onSubmit={onSubmit} isUpdating={false} disabled={false} />,
		);
		await user.type(screen.getByLabelText(/^name/i), name);
		await user.type(
			screen.getByLabelText(/callback url/i),
			"https://example.com/callback",
		);
		await user.click(
			screen.getByRole("button", { name: /create application/i }),
		);
		await act(async () => {});
		if (valid) {
			await waitFor(() =>
				expect(onSubmit).toHaveBeenCalledWith({
					name,
					callback_url: "https://example.com/callback",
					icon: "",
				}),
			);
		} else {
			expect(onSubmit).not.toHaveBeenCalled();
		}
	});
});
