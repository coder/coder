import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
	MockOAuth2ProviderAppPublic,
	MockOAuth2ProviderApps,
} from "#/testHelpers/entities";
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
			redirect_uris: ["vscode://coder.coder-remote/oauth/callback"],
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
		"localhost:3000",
		"vscode:",
		"a:",
		"vscode://",
		"mailto:a@b",
		"tel:+1234",
		"sms:+1234",
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
		"com.example.app:/oauth2redirect",
		"cursor://anysphere.cursor-mcp/oauth/callback",
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

	it.each([
		{ callback: "mailto:a@b", valid: false },
		{ callback: "mailto://a@b", valid: false },
		{ callback: "tel:+1234", valid: false },
		{ callback: "tel:/1234", valid: false },
		{ callback: "sms:+1234", valid: false },
		{ callback: "sms:/1234", valid: false },
		{ callback: "http://example.com/callback", valid: false },
		{ callback: "https://example.com/callback#fragment", valid: false },
		{ callback: "http://localhost:3000/callback", valid: true },
		{ callback: "http://127.0.0.1:3000/callback", valid: true },
		{ callback: "http://[::1]:3000/callback", valid: true },
		{ callback: "http://app.localhost/callback", valid: false },
		{ callback: "https://example.com/callback", valid: true },
		{ callback: "vscode://coder.coder-remote/oauth/callback", valid: true },
		{ callback: "com.example.app:/oauth2redirect", valid: true },
	])(
		"validates public client callback $callback",
		async ({ callback, valid }) => {
			const user = userEvent.setup();
			const onSubmit = vi.fn();
			render(
				<OAuth2AppForm
					app={MockOAuth2ProviderAppPublic}
					onSubmit={onSubmit}
					isUpdating={false}
					disabled={false}
				/>,
			);
			await user.clear(screen.getByLabelText(/callback url/i));
			await user.type(
				screen.getByLabelText(/callback url/i),
				callback.replaceAll("[", "[["),
			);
			await user.type(screen.getByLabelText(/^name/i), " updated");
			await user.click(
				screen.getByRole("button", { name: /update application/i }),
			);
			await act(async () => {});
			if (valid) {
				await waitFor(() =>
					expect(onSubmit).toHaveBeenCalledWith({
						name: `${MockOAuth2ProviderAppPublic.name} updated`,
						callback_url: callback,
						icon: MockOAuth2ProviderAppPublic.icon,
					}),
				);
			} else {
				expect(onSubmit).not.toHaveBeenCalled();
			}
		},
	);

	it.each([
		{ callback: "http://example.com/callback", valid: false },
		{ callback: "http://10.0.0.5:8080/callback", valid: false },
		{ callback: "http://localhost:3000/callback", valid: true },
		{ callback: "http://127.0.0.1:3000/callback", valid: true },
		{ callback: "http://[::1]:3000/callback", valid: true },
		{ callback: "http://app.localhost/callback", valid: true },
		{ callback: "https://example.com/callback", valid: true },
		{ callback: "vscode://coder.coder-remote/oauth/callback", valid: true },
	])(
		"validates confidential client callback $callback",
		async ({ callback, valid }) => {
			const user = userEvent.setup();
			const onSubmit = vi.fn();
			render(
				<OAuth2AppForm
					onSubmit={onSubmit}
					isUpdating={false}
					disabled={false}
				/>,
			);
			await user.type(screen.getByLabelText(/^name/i), "confidential-app");
			await user.type(
				screen.getByLabelText(/callback url/i),
				callback.replaceAll("[", "[["),
			);
			await user.click(
				screen.getByRole("button", { name: /create application/i }),
			);
			await act(async () => {});
			if (valid) {
				await waitFor(() =>
					expect(onSubmit).toHaveBeenCalledWith({
						name: "confidential-app",
						callback_url: callback,
						icon: "",
					}),
				);
			} else {
				expect(onSubmit).not.toHaveBeenCalled();
			}
		},
	);
});
