import { act, screen, waitFor } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { useState } from "react";
import { API } from "#/api/api";
import {
	OAuth2RedirectURIMaxBytes,
	OAuth2RedirectURIsMaxCount,
	OAuth2ScopeListMaxBytes,
	OAuth2ScopeListMaxNames,
} from "#/api/typesGenerated";
import {
	MockExternalAPIKeyScopes,
	MockOAuth2ProviderAppPublic,
	MockOAuth2ProviderApps,
} from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { narrowsAllowlist, OAuth2AppForm } from "./OAuth2AppForm";

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
			<OAuth2AppForm
				clientType="confidential"
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
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
				scope: "",
			});
		});
	});

	it("trims the edited name on update", async () => {
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
				clientType={app.client_type}
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
			<OAuth2AppForm
				clientType="confidential"
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
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
		" https://example.com/callback",
		"https://example.com/callback ",
		" https://example.com/callback ",
		" http://localhost:3000/callback",
		"http://127.0.0.1:3000/callback ",
		"URN:ietf:wg:oauth:2.0:oob",
		"com.example.app:/oauth2redirect",
		"cursor://anysphere.cursor-mcp/oauth/callback",
	])("submits trimmed callback %j", async (callback) => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		render(
			<OAuth2AppForm
				clientType="confidential"
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
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
				scope: "",
			}),
		);
	});

	it.each([
		{ name: "é".repeat(32), valid: true },
		{ name: `a${"é".repeat(32)}`, valid: false },
		{ name: "é".repeat(33), valid: false },
		{ name: "界".repeat(21), valid: true },
		{ name: "界".repeat(22), valid: false },
	])("enforces the UTF-8 byte limit for $name", async ({ name, valid }) => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		render(
			<OAuth2AppForm
				clientType="confidential"
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
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
					scope: "",
				}),
			);
		} else {
			expect(onSubmit).not.toHaveBeenCalled();
		}
	});

	// Each "é" is two UTF-8 bytes but one UTF-16 code unit, so these URIs stay
	// well under the limit by string length and only cross it by byte count.
	const redirectURIPrefix = "https://example.com/";
	const redirectURIOfBytes = (bytes: number) =>
		redirectURIPrefix +
		"é".repeat(Math.floor((bytes - redirectURIPrefix.length) / 2)) +
		"a".repeat((bytes - redirectURIPrefix.length) % 2);

	it.each([
		{ bytes: OAuth2RedirectURIMaxBytes, valid: true },
		{ bytes: OAuth2RedirectURIMaxBytes + 1, valid: false },
	])(
		"enforces the UTF-8 byte limit for a $bytes byte redirect URI",
		async ({ bytes, valid }) => {
			const uri = redirectURIOfBytes(bytes);
			expect(new TextEncoder().encode(uri)).toHaveLength(bytes);
			expect(uri.length).toBeLessThan(OAuth2RedirectURIMaxBytes);

			const user = userEvent.setup();
			const onSubmit = vi.fn();
			render(
				<OAuth2AppForm
					clientType="confidential"
					onSubmit={onSubmit}
					isUpdating={false}
					disabled={false}
				/>,
			);
			await user.type(screen.getByLabelText(/^name/i), "OAuth App");
			await user.click(screen.getByLabelText(/default callback/i));
			await user.paste(uri);
			await user.click(
				screen.getByRole("button", { name: /create application/i }),
			);
			await act(async () => {});
			if (valid) {
				await waitFor(() =>
					expect(onSubmit).toHaveBeenCalledWith({
						name: "OAuth App",
						redirect_uris: [uri],
						icon: "",
						scope: "",
					}),
				);
			} else {
				expect(onSubmit).not.toHaveBeenCalled();
			}
		},
	);

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
					clientType="public"
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
					clientType="confidential"
					onSubmit={onSubmit}
					isUpdating={false}
					disabled={false}
				/>,
			);
			await user.type(screen.getByLabelText(/^name/i), "confidential-app");
			await user.type(
				screen.getByLabelText(/default callback/i),
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
						redirect_uris: [callback],
						icon: "",
						scope: "",
					}),
				);
			} else {
				expect(onSubmit).not.toHaveBeenCalled();
			}
		},
	);

	// The stored list is left out of an unrelated save. If another admin removed
	// a URI after this form loaded, resending the loaded list would restore it.
	// The second save runs after the app prop refreshes to the server's list, as
	// it does on the edit page, while the form still holds the loaded list.
	it("omits redirect_uris and callback_url on repeated renames", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const loadedApp = {
			...MockOAuth2ProviderApps[0],
			redirect_uris: ["https://a.example.com/cb", "https://b.example.com/cb"],
		};
		const serverRedirectURIs = ["https://a.example.com/cb"];
		const EditPage = () => {
			const [app, setApp] = useState(loadedApp);
			return (
				<OAuth2AppForm
					app={app}
					clientType={app.client_type}
					onSubmit={(req) => {
						onSubmit(req);
						setApp({ ...app, ...req, redirect_uris: serverRedirectURIs });
					}}
					isUpdating={false}
					disabled={false}
				/>
			);
		};

		render(<EditPage />);

		for (const name of ["Renamed app", "Renamed again"]) {
			await user.clear(screen.getByLabelText(/^name/i));
			await user.type(screen.getByLabelText(/^name/i), name);
			await user.click(
				screen.getByRole("button", { name: /update application/i }),
			);
			await waitFor(() =>
				expect(onSubmit).toHaveBeenLastCalledWith({
					name,
					icon: loadedApp.icon,
				}),
			);
		}
		expect(onSubmit).toHaveBeenCalledTimes(2);
	});

	// A finished save resets the form to the untrimmed input. Comparing the
	// trimmed submission against that input would resend the list on the next
	// unrelated save and restore a URI another admin removed in between.
	it("omits redirect_uris on a rename after saving a padded URI", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const loadedApp = {
			...MockOAuth2ProviderApps[0],
			redirect_uris: ["https://a.example.com/cb", "https://b.example.com/cb"],
		};
		const serverRedirectURIs = ["https://c.example.com/cb"];
		let finishSave = () => {};
		const EditPage = () => {
			const [app, setApp] = useState(loadedApp);
			const [isUpdating, setIsUpdating] = useState(false);
			return (
				<OAuth2AppForm
					app={app}
					clientType={app.client_type}
					onSubmit={(req) => {
						onSubmit(req);
						setIsUpdating(true);
						return new Promise((resolve) => {
							finishSave = () => {
								setApp({ ...app, ...req, redirect_uris: serverRedirectURIs });
								setIsUpdating(false);
								resolve();
							};
						});
					}}
					isUpdating={isUpdating}
					disabled={false}
				/>
			);
		};

		render(<EditPage />);

		await user.clear(screen.getByLabelText(/default callback/i));
		await user.type(
			screen.getByLabelText(/default callback/i),
			" https://c.example.com/cb ",
		);
		await user.click(
			screen.getByRole("button", { name: /update application/i }),
		);
		await waitFor(() =>
			expect(onSubmit).toHaveBeenLastCalledWith({
				name: loadedApp.name,
				redirect_uris: ["https://c.example.com/cb", "https://b.example.com/cb"],
				icon: loadedApp.icon,
			}),
		);
		await act(async () => finishSave());

		await user.clear(screen.getByLabelText(/^name/i));
		await user.type(screen.getByLabelText(/^name/i), "Renamed app");
		await user.click(
			screen.getByRole("button", { name: /update application/i }),
		);
		await waitFor(() =>
			expect(onSubmit).toHaveBeenLastCalledWith({
				name: "Renamed app",
				icon: loadedApp.icon,
			}),
		);
		expect(onSubmit).toHaveBeenCalledTimes(2);
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
				clientType={app.client_type}
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
				clientType={app.client_type}
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
				clientType={app.client_type}
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
				clientType={app.client_type}
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
		expect(
			screen.getByText(/at least one redirect uri is required/i),
		).toBeInTheDocument();
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
				clientType={app.client_type}
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

	it("does not submit when a row duplicates another row", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const app = {
			...MockOAuth2ProviderApps[0],
			redirect_uris: ["https://a.example.com/cb", "https://b.example.com/cb"],
		};

		render(
			<OAuth2AppForm
				app={app}
				clientType={app.client_type}
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
		);

		await user.clear(screen.getByLabelText(/^redirect uri 2/i));
		await user.type(
			screen.getByLabelText(/^redirect uri 2/i),
			"https://a.example.com/cb",
		);
		await user.click(
			screen.getByRole("button", { name: /update application/i }),
		);

		await act(async () => {});
		expect(onSubmit).not.toHaveBeenCalled();
		expect(
			screen.getByText(/already used by another row/i),
		).toBeInTheDocument();
	});

	it("flags every row that duplicates an earlier one", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		const app = {
			...MockOAuth2ProviderApps[0],
			redirect_uris: [
				"https://a.example.com/cb",
				"https://b.example.com/cb",
				"https://c.example.com/cb",
			],
		};

		render(
			<OAuth2AppForm
				app={app}
				clientType={app.client_type}
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
		);

		for (const row of [2, 3]) {
			const field = screen.getByLabelText(
				new RegExp(`^redirect uri ${row}`, "i"),
			);
			await user.clear(field);
			await user.type(field, "https://a.example.com/cb");
		}
		await user.click(
			screen.getByRole("button", { name: /update application/i }),
		);

		await act(async () => {});
		expect(onSubmit).not.toHaveBeenCalled();
		expect(screen.getAllByText(/already used by another row/i)).toHaveLength(2);
	});

	// Confidential apps could store a non-local http redirect URI before the
	// form checked for it. The stored value fails validation on load, so the
	// error must show without the field being touched.
	it("shows the error for a stored redirect URI that fails validation", async () => {
		const user = userEvent.setup();
		const onSubmit = vi.fn();
		render(
			<OAuth2AppForm
				app={{
					...MockOAuth2ProviderApps[0],
					redirect_uris: ["http://intranet.example.com/callback"],
				}}
				clientType="confidential"
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
		);

		expect(
			await screen.findByText("Please enter a valid redirect URI."),
		).toBeInTheDocument();
		await user.type(screen.getByLabelText(/^name/i), " updated");
		expect(
			screen.getByRole("button", { name: /update application/i }),
		).toBeDisabled();

		await user.clear(screen.getByLabelText(/default callback/i));
		await user.type(
			screen.getByLabelText(/default callback/i),
			"https://intranet.example.com/callback",
		);
		await waitFor(() =>
			expect(
				screen.queryByText("Please enter a valid redirect URI."),
			).not.toBeInTheDocument(),
		);
		await user.click(
			screen.getByRole("button", { name: /update application/i }),
		);
		await waitFor(() =>
			expect(onSubmit).toHaveBeenCalledWith({
				name: `${MockOAuth2ProviderApps[0].name} updated`,
				redirect_uris: ["https://intranet.example.com/callback"],
				icon: MockOAuth2ProviderApps[0].icon,
			}),
		);
	});

	// A list over the count cap has no row to blur, so its message must also
	// show on load.
	it("shows the error for a stored list over the count cap", async () => {
		render(
			<OAuth2AppForm
				app={{
					...MockOAuth2ProviderApps[0],
					redirect_uris: Array.from(
						{ length: OAuth2RedirectURIsMaxCount + 1 },
						(_, i) => `https://alt-${i}.example.com/cb`,
					),
				}}
				clientType="confidential"
				onSubmit={vi.fn()}
				isUpdating={false}
				disabled={false}
			/>,
		);

		expect(
			await screen.findByText(
				`At most ${OAuth2RedirectURIsMaxCount} redirect URIs are allowed.`,
			),
		).toBeInTheDocument();
		expect(
			screen.getByRole("button", { name: /update application/i }),
		).toBeDisabled();
	});

	it("keeps the redirect URI error hidden when the stored list is valid", async () => {
		const user = userEvent.setup();
		render(
			<OAuth2AppForm
				app={MockOAuth2ProviderApps[0]}
				clientType="confidential"
				onSubmit={vi.fn()}
				isUpdating={false}
				disabled={false}
			/>,
		);

		await user.type(screen.getByLabelText(/^name/i), " updated");
		expect(
			screen.queryByText("Please enter a valid redirect URI."),
		).not.toBeInTheDocument();
		expect(
			screen.getByRole("button", { name: /update application/i }),
		).toBeEnabled();
	});

	it("submits the selected scopes as a space separated list", async () => {
		const onSubmit = vi.fn();
		const user = userEvent.setup();
		render(
			<OAuth2AppForm
				clientType="confidential"
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
		);

		await user.type(screen.getByLabelText(/^name/i), "test-app");
		await user.type(
			screen.getByLabelText(/default callback/i),
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
				redirect_uris: ["https://example.com/callback"],
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
				clientType="confidential"
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
				clientType="confidential"
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

	// The stored list is left out of a name-only update because the form cannot
	// resend it faithfully. Whitespace-only would come back as "" and lift the
	// restriction, and oversized legacy lists fail the current size limits.
	const untouchedScopes = [
		{
			label: "a scope the catalog does not list",
			scope: "legacy:scope coder:all",
		},
		{ label: "a whitespace-only scope", scope: "   " },
		{
			label: "more names than the limit allows",
			scope: Array.from(
				{ length: OAuth2ScopeListMaxNames + 1 },
				(_, i) => `legacy:scope${i}`,
			).join(" "),
		},
		{
			label: "a scope longer than the byte limit",
			scope: `legacy:${"a".repeat(OAuth2ScopeListMaxBytes)}`,
		},
	];

	it.each(untouchedScopes)(
		"omits $label from a name-only update",
		async ({ scope }) => {
			const onSubmit = vi.fn();
			const user = userEvent.setup();
			render(
				<OAuth2AppForm
					app={{ ...MockOAuth2ProviderApps[0], scope }}
					clientType="confidential"
					onSubmit={onSubmit}
					isUpdating={false}
					disabled={false}
				/>,
			);

			await user.type(screen.getByLabelText(/^name/i), "-updated");
			await user.click(
				screen.getByRole("button", { name: /update application/i }),
			);

			await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
			expect(onSubmit.mock.calls[0][0]).toStrictEqual({
				name: "foo-updated",
				icon: MockOAuth2ProviderApps[0].icon,
			});
		},
	);

	it("omits a whitespace-only scope when the catalog fails to load", async () => {
		vi.spyOn(API, "getExternalAPIKeyScopes").mockRejectedValue(
			new Error("catalog unavailable"),
		);
		const onSubmit = vi.fn();
		const user = userEvent.setup();
		render(
			<OAuth2AppForm
				app={{ ...MockOAuth2ProviderApps[0], scope: "   " }}
				clientType="confidential"
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
		);

		await user.type(screen.getByLabelText(/^name/i), "-updated");
		await user.click(
			screen.getByRole("button", { name: /update application/i }),
		);

		await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
		expect(onSubmit.mock.calls[0][0]).toStrictEqual({
			name: "foo-updated",
			icon: MockOAuth2ProviderApps[0].icon,
		});
	});

	it("sends the scope when a selection is added to an existing allowlist", async () => {
		const onSubmit = vi.fn();
		const user = userEvent.setup();
		render(
			<OAuth2AppForm
				app={{ ...MockOAuth2ProviderApps[0], scope: "legacy:scope" }}
				clientType="confidential"
				onSubmit={onSubmit}
				isUpdating={false}
				disabled={false}
			/>,
		);

		await selectScope(user, "workspace:ssh");
		await user.click(
			screen.getByRole("button", { name: /update application/i }),
		);

		await waitFor(() =>
			expect(onSubmit).toHaveBeenCalledWith(
				expect.objectContaining({ scope: "legacy:scope workspace:ssh" }),
			),
		);
	});
});

describe("narrowsAllowlist", () => {
	const catalog = MockExternalAPIKeyScopes.external;

	it.each([
		{
			label: "adding a scope to an existing list",
			stored: "workspace:ssh",
			next: ["workspace:ssh", "workspace:read"],
			narrows: false,
		},
		{
			label: "removing a scope from an existing list",
			stored: "workspace:ssh workspace:read",
			next: ["workspace:read"],
			narrows: true,
		},
		{
			label: "swapping one scope for another",
			stored: "workspace:ssh",
			next: ["workspace:read"],
			narrows: true,
		},
		{
			label: "clearing the list",
			stored: "workspace:ssh",
			next: [],
			narrows: false,
		},
		{
			label: "imposing a list on an unrestricted app",
			stored: "",
			next: ["workspace:ssh"],
			narrows: true,
		},
		{
			label: "imposing a list of unknown names on an unrestricted app",
			stored: "",
			next: ["legacy:thing"],
			narrows: true,
		},
		{
			label: "replacing a scope with coder:all",
			stored: "workspace:ssh",
			next: ["coder:all"],
			narrows: false,
		},
		{
			label: "imposing coder:all on an unrestricted app",
			stored: "",
			next: ["coder:all"],
			narrows: false,
		},
		{
			label: "keeping the same list",
			stored: "workspace:ssh",
			next: ["workspace:ssh"],
			narrows: false,
		},
		{
			label: "removing a name the deployment does not offer",
			stored: "workspace:ssh legacy:thing",
			next: ["workspace:ssh"],
			narrows: false,
		},
		{
			label: "removing the only offered name",
			stored: "workspace:ssh legacy:thing",
			next: ["legacy:thing"],
			narrows: true,
		},
		{
			label: "replacing a list that grants nothing",
			stored: "legacy:thing",
			next: ["workspace:read"],
			narrows: false,
		},
		{
			label: "adding a scope to a whitespace-only list",
			stored: "   ",
			next: ["workspace:read"],
			narrows: false,
		},
	])("returns $narrows when $label", ({ stored, next, narrows }) => {
		expect(narrowsAllowlist(stored, next, catalog)).toBe(narrows);
	});
});
