import { act, screen, waitFor } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { API } from "#/api/api";
import {
	MockExternalAPIKeyScopes,
	MockOAuth2ProviderAppPublic,
	MockOAuth2ProviderApps,
} from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { OAuth2AppForm } from "./OAuth2AppForm";

const selectScope = async (user: UserEvent, name: string) => {
	await user.click(screen.getByRole("combobox", { name: /allowed scopes/i }));
	await user.click(await screen.findByRole("option", { name }));
};

describe("OAuth2AppForm", () => {
	// The form loads the scope catalog on mount. Tests that exercise a failed
	// load override this spy.
	beforeEach(() => {
		vi.spyOn(API, "getExternalAPIKeyScopes").mockResolvedValue(
			MockExternalAPIKeyScopes,
		);
	});

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
				scope: "",
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
				scope: "",
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
				scope: "",
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
					scope: "",
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
						scope: "",
					}),
				);
			} else {
				expect(onSubmit).not.toHaveBeenCalled();
			}
		},
	);

	it("submits the selected scopes as a space separated list", async () => {
		const onSubmit = vi.fn();
		const user = userEvent.setup();
		render(
			<OAuth2AppForm onSubmit={onSubmit} isUpdating={false} disabled={false} />,
		);

		await user.type(screen.getByLabelText(/^name/i), "test-app");
		await user.type(
			screen.getByLabelText(/callback url/i),
			"https://example.com/callback",
		);
		await selectScope(user, "workspace:ssh");
		await selectScope(user, "coder:all");
		await user.click(
			screen.getByRole("button", { name: /create application/i }),
		);

		await waitFor(() =>
			expect(onSubmit).toHaveBeenCalledWith({
				name: "test-app",
				callback_url: "https://example.com/callback",
				icon: "",
				scope: "workspace:ssh coder:all",
			}),
		);
	});

	it("submits an empty scope when the selection is cleared", async () => {
		const onSubmit = vi.fn();
		const user = userEvent.setup();
		render(
			<OAuth2AppForm
				app={{ ...MockOAuth2ProviderApps[0], scope: "coder:all" }}
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
		);

		await user.click(screen.getByTestId("clear-all-button"));
		await user.click(
			screen.getByRole("button", { name: /update application/i }),
		);

		await waitFor(() =>
			expect(onSubmit).toHaveBeenCalledWith(
				expect.objectContaining({ scope: "" }),
			),
		);
	});

	it("lets the admin retry a failed catalog load and then pick a scope", async () => {
		vi.spyOn(API, "getExternalAPIKeyScopes")
			.mockRejectedValueOnce(new Error("catalog unavailable"))
			.mockResolvedValueOnce(MockExternalAPIKeyScopes);
		const onSubmit = vi.fn();
		const user = userEvent.setup();
		render(
			<OAuth2AppForm
				app={MockOAuth2ProviderApps[0]}
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
		);

		await user.click(await screen.findByRole("button", { name: /retry/i }));
		await selectScope(user, "workspace:ssh");
		await user.click(
			screen.getByRole("button", { name: /update application/i }),
		);

		await waitFor(() =>
			expect(onSubmit).toHaveBeenCalledWith(
				expect.objectContaining({ scope: "workspace:ssh" }),
			),
		);
	});

	it("keeps a configured scope the catalog does not list", async () => {
		const onSubmit = vi.fn();
		const user = userEvent.setup();
		render(
			<OAuth2AppForm
				app={{ ...MockOAuth2ProviderApps[0], scope: "legacy:scope coder:all" }}
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
		);

		await user.type(screen.getByLabelText(/^name/i), "-updated");
		await user.click(
			screen.getByRole("button", { name: /update application/i }),
		);

		await waitFor(() =>
			expect(onSubmit).toHaveBeenCalledWith(
				expect.objectContaining({
					name: "foo-updated",
					scope: "legacy:scope coder:all",
				}),
			),
		);
	});

	it("keeps the configured scopes when the catalog fails to load", async () => {
		vi.spyOn(API, "getExternalAPIKeyScopes").mockRejectedValue(
			new Error("catalog unavailable"),
		);
		const onSubmit = vi.fn();
		const user = userEvent.setup();
		render(
			<OAuth2AppForm
				app={{ ...MockOAuth2ProviderApps[0], scope: "coder:all workspace:ssh" }}
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
		);

		await user.type(screen.getByLabelText(/^name/i), "-updated");
		await user.click(
			screen.getByRole("button", { name: /update application/i }),
		);

		await waitFor(() =>
			expect(onSubmit).toHaveBeenCalledWith(
				expect.objectContaining({
					name: "foo-updated",
					scope: "coder:all workspace:ssh",
				}),
			),
		);
	});
});
