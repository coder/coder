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
			screen.getByLabelText(/default callback/i),
			"vscode://coder.coder-remote/oauth/callback",
		);
		await user.click(
			screen.getByRole("button", { name: /create application/i }),
		);

		await waitFor(() => {
			expect(onSubmit).toHaveBeenCalledWith({
				name: "VS Code Coder Extension",
				redirect_uris: ["vscode://coder.coder-remote/oauth/callback"],
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
				redirect_uris: ["vscode://coder.coder-remote/oauth/callback"],
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
		const callbackURL = screen.getByLabelText(/default callback/i);
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
		await user.type(screen.getByLabelText(/default callback/i), callback);
		await user.click(
			screen.getByRole("button", { name: /create application/i }),
		);
		await waitFor(() =>
			expect(onSubmit).toHaveBeenCalledWith({
				name: "OAuth App",
				redirect_uris: [callback.trim()],
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
			screen.getByLabelText(/default callback/i),
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
					redirect_uris: ["https://example.com/callback"],
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
			await user.clear(screen.getByLabelText(/default callback/i));
			await user.type(
				screen.getByLabelText(/default callback/i),
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
						redirect_uris: [callback],
						icon: MockOAuth2ProviderAppPublic.icon,
					}),
				);
			} else {
				expect(onSubmit).not.toHaveBeenCalled();
			}
		},
	);

	it("sends the list unchanged and no callback_url key on rename", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const app = {
			...MockOAuth2ProviderApps[0],
			redirect_uris: ["https://a.example.com/cb", "https://b.example.com/cb"],
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
		await user.type(screen.getByLabelText(/^name/i), "Renamed app");
		await user.click(
			screen.getByRole("button", { name: /update application/i }),
		);

		await waitFor(() => {
			const call = onSubmit.mock.calls[0][0];
			expect(call).toStrictEqual({
				name: "Renamed app",
				redirect_uris: ["https://a.example.com/cb", "https://b.example.com/cb"],
				icon: app.icon,
			});
			expect(call).not.toHaveProperty("callback_url");
		});
	});

	it("edits an entry in place, keeping its position", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const app = {
			...MockOAuth2ProviderApps[0],
			redirect_uris: ["https://a.example.com/cb", "https://b.example.com/cb"],
		};

		render(
			<OAuth2AppForm
				app={app}
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
		);

		await user.clear(screen.getByLabelText(/default callback/i));
		await user.type(
			screen.getByLabelText(/default callback/i),
			"https://c.example.com/cb",
		);
		await user.click(
			screen.getByRole("button", { name: /update application/i }),
		);

		await waitFor(() =>
			expect(onSubmit).toHaveBeenCalledWith({
				name: app.name,
				redirect_uris: ["https://c.example.com/cb", "https://b.example.com/cb"],
				icon: app.icon,
			}),
		);
	});

	it("removes an entry", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const app = {
			...MockOAuth2ProviderApps[0],
			redirect_uris: ["https://a.example.com/cb", "https://b.example.com/cb"],
		};

		render(
			<OAuth2AppForm
				app={app}
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
		);

		await user.click(
			screen.getByRole("button", { name: /remove redirect uri 2/i }),
		);
		await user.click(
			screen.getByRole("button", { name: /update application/i }),
		);

		await waitFor(() =>
			expect(onSubmit).toHaveBeenCalledWith({
				name: app.name,
				redirect_uris: ["https://a.example.com/cb"],
				icon: app.icon,
			}),
		);
	});

	it("adds an entry", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const app = {
			...MockOAuth2ProviderApps[0],
			redirect_uris: ["https://a.example.com/cb", "https://b.example.com/cb"],
		};

		render(
			<OAuth2AppForm
				app={app}
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
		);

		await user.click(screen.getByRole("button", { name: /add redirect uri/i }));
		await user.type(
			screen.getByLabelText(/^redirect uri 3/i),
			"https://d.example.com/cb",
		);
		await user.click(
			screen.getByRole("button", { name: /update application/i }),
		);

		await waitFor(() =>
			expect(onSubmit).toHaveBeenCalledWith({
				name: app.name,
				redirect_uris: [
					"https://a.example.com/cb",
					"https://b.example.com/cb",
					"https://d.example.com/cb",
				],
				icon: app.icon,
			}),
		);
	});

	it("disables submit after removing the only entry", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const app = {
			...MockOAuth2ProviderApps[0],
			redirect_uris: ["https://a.example.com/cb"],
		};

		render(
			<OAuth2AppForm
				app={app}
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
		);

		await user.click(
			screen.getByRole("button", { name: /remove redirect uri 1/i }),
		);

		await waitFor(() =>
			expect(
				screen.getByRole("button", { name: /update application/i }),
			).toBeDisabled(),
		);
	});

	it("does not submit when a public app has a disallowed entry in another row", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const app = {
			...MockOAuth2ProviderAppPublic,
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

		await user.click(screen.getByRole("button", { name: /add redirect uri/i }));
		await user.type(
			screen.getByLabelText(/^redirect uri 2/i),
			"http://example.com/cb",
		);
		await user.click(
			screen.getByRole("button", { name: /update application/i }),
		);

		await act(async () => {});
		expect(onSubmit).not.toHaveBeenCalled();
	});
});
