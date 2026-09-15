import { screen, waitFor } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { API } from "#/api/api";
import {
	MockExternalAPIKeyScopes,
	MockOAuth2ProviderApps,
} from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import { OAuth2AppForm } from "./OAuth2AppForm";

const selectScope = async (user: UserEvent, name: string) => {
	await user.click(screen.getByRole("combobox", { name: /allowed scopes/i }));
	await user.click(await screen.findByRole("option", { name }));
};

describe("OAuth2AppForm", () => {
	it("submits the selected scopes as a space separated list", async () => {
		vi.spyOn(API, "getExternalAPIKeyScopes").mockResolvedValue(
			MockExternalAPIKeyScopes,
		);
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
		vi.spyOn(API, "getExternalAPIKeyScopes").mockResolvedValue(
			MockExternalAPIKeyScopes,
		);
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
		vi.spyOn(API, "getExternalAPIKeyScopes").mockResolvedValue(
			MockExternalAPIKeyScopes,
		);
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
