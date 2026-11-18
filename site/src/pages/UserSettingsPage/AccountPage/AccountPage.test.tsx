import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API, withDefaultFeatures } from "#/api/api";
import { preferenceSettingsKey } from "#/api/queries/users";
import type { UserPreferenceSettings } from "#/api/typesGenerated";
import { defaultMetadataManager } from "#/hooks/useEmbeddedMetadata";
import { DashboardIndexRedirect } from "#/modules/dashboard/DashboardIndexRedirect";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockEntitlements,
	MockGroup,
	MockPermissions,
	MockUserOwner,
	MockUserPreferenceSettings,
	mockApiError,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import AccountPage from "./AccountPage";

const newData = {
	username: "user",
	name: "Mr User",
	avatar_url: MockUserOwner.avatar_url,
};

const fillAndSubmitForm = async () => {
	await waitFor(() => screen.findByLabelText("Username"));
	fireEvent.change(screen.getByLabelText("Username"), {
		target: { value: newData.username },
	});
	await waitFor(() => screen.findByLabelText("Name"));
	fireEvent.change(screen.getByLabelText("Name"), {
		target: { value: newData.name },
	});
	fireEvent.click(screen.getByText("Update account"));
};

describe("AccountPage", () => {
	it("sets a page title that identifies the page", async () => {
		renderWithAuth(<AccountPage />);
		await waitFor(() => {
			expect(document.title).toBe("Account - Settings - Coder");
		});
	});

	it("nests dashboard and groups headings under the page heading", async () => {
		vi.spyOn(API, "getEntitlements").mockResolvedValue({
			...MockEntitlements,
			features: withDefaultFeatures({
				user_role_management: { enabled: true, entitlement: "entitled" },
			}),
		});
		vi.spyOn(API, "getGroups").mockResolvedValue([MockGroup]);
		renderWithAuth(<AccountPage />);

		await screen.findByText(MockGroup.display_name);
		expect(
			screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent),
		).toEqual(["Dashboard", "Your groups"]);
	});

	describe("default landing page", () => {
		afterEach(() => vi.restoreAllMocks());

		it.each([
			{ initial: "workspaces", next: "agents", label: "Agents" },
			{ initial: "agents", next: "workspaces", label: "Workspaces" },
		] as const)(
			"uses saved $next instead of embedded preferences when reloading fails",
			async ({ initial, next, label }) => {
				const user = userEvent.setup();
				const settings = {
					...MockUserPreferenceSettings,
					default_landing_page: initial,
				};
				const updated = { ...settings, default_landing_page: next };
				const metadata = defaultMetadataManager.getMetadata();
				vi.spyOn(defaultMetadataManager, "getMetadata").mockReturnValue({
					...metadata,
					userPreferences: { available: true, value: settings },
				});
				const getPreference = vi
					.spyOn(API, "getUserPreferenceSettings")
					.mockResolvedValue(settings);
				const update = vi
					.spyOn(API, "updateUserPreferenceSettings")
					.mockResolvedValue(updated);
				const { queryClient, router } = renderWithAuth(<AccountPage />, {
					path: "/settings/account",
					route: "/settings/account",
					extraRoutes: [
						{ path: "/", element: <DashboardIndexRedirect /> },
						{ path: "/agents", element: <div>Agents</div> },
						{ path: "/workspaces", element: <div>Workspaces</div> },
					],
				});
				await screen.findByText(initial === "agents" ? "Agents" : "Workspaces");
				expect(getPreference).not.toHaveBeenCalled();
				getPreference.mockRejectedValue(new Error("Preferences unavailable"));

				await user.click(
					screen.getByRole("combobox", { name: "Default landing page" }),
				);
				await user.click(await screen.findByRole("option", { name: label }));

				await waitFor(() => {
					expect(queryClient.getQueryState(preferenceSettingsKey)?.status).toBe(
						"error",
					);
				});
				expect(queryClient.getQueryData(preferenceSettingsKey)).toEqual(
					updated,
				);
				expect(update).toHaveBeenCalledExactlyOnceWith({
					default_landing_page: next,
				});

				await act(() => router.navigate("/"));
				await waitFor(() => {
					expect(router.state.location.pathname).toBe(`/${next}`);
				});
				expect(getPreference).toHaveBeenCalledTimes(1);
			},
		);

		it("keeps keyboard focus while saving and ignores repeated selections", async () => {
			const user = userEvent.setup();
			const saved = createDeferred<UserPreferenceSettings>();
			const getPreference = vi
				.spyOn(API, "getUserPreferenceSettings")
				.mockResolvedValue(MockUserPreferenceSettings);
			const update = vi
				.spyOn(API, "updateUserPreferenceSettings")
				.mockReturnValue(saved.promise);
			const { queryClient } = renderWithAuth(<AccountPage />);
			await screen.findByText("Workspaces");
			const trigger = screen.getByRole("combobox", {
				name: "Default landing page",
			});
			const status = screen.getByRole("status");
			expect(status.textContent).toBe("");
			await user.click(trigger);
			await user.click(await screen.findByRole("option", { name: "Agents" }));
			await waitFor(() =>
				expect(status).toHaveTextContent("Saving default landing page"),
			);
			await waitFor(() => expect(document.activeElement).toBe(trigger));

			await user.keyboard("{Enter}{ArrowUp}{Enter}");
			expect(document.activeElement).toBe(trigger);
			expect(update).toHaveBeenCalledExactlyOnceWith({
				default_landing_page: "agents",
			});

			const updated: UserPreferenceSettings = {
				...MockUserPreferenceSettings,
				default_landing_page: "agents",
			};
			getPreference.mockResolvedValue(updated);
			saved.resolve(updated);
			await waitFor(() => {
				expect(queryClient.getQueryData(preferenceSettingsKey)).toEqual(
					updated,
				);
			});
			expect(document.activeElement).toBe(trigger);
			await waitFor(() =>
				expect(status).toHaveTextContent("Default landing page saved"),
			);
		});

		it("keeps the saved preference after a failed update and allows retrying", async () => {
			const user = userEvent.setup();
			const updated: UserPreferenceSettings = {
				...MockUserPreferenceSettings,
				default_landing_page: "agents",
			};
			vi.spyOn(API, "getUserPreferenceSettings")
				.mockResolvedValueOnce(MockUserPreferenceSettings)
				.mockResolvedValue(updated);
			const update = vi
				.spyOn(API, "updateUserPreferenceSettings")
				.mockRejectedValueOnce(
					mockApiError({ message: "Preference could not be saved." }),
				)
				.mockResolvedValue(updated);
			const { queryClient } = renderWithAuth(<AccountPage />);
			await screen.findByText("Workspaces");
			const trigger = screen.getByRole("combobox", {
				name: "Default landing page",
			});
			await user.click(trigger);
			await user.click(await screen.findByRole("option", { name: "Agents" }));
			await screen.findByText("Preference could not be saved.");
			expect(trigger).toHaveAccessibleDescription(
				/Choose your dashboard home page.*Preference could not be saved\./,
			);

			expect(queryClient.getQueryData(preferenceSettingsKey)).toEqual(
				MockUserPreferenceSettings,
			);
			await user.click(trigger);
			await user.click(await screen.findByRole("option", { name: "Agents" }));
			await waitFor(() => {
				expect(queryClient.getQueryData(preferenceSettingsKey)).toEqual(
					updated,
				);
			});
			expect(trigger).toHaveAccessibleDescription(
				"Choose your dashboard home page. This preference syncs across your devices.",
			);
			expect(update.mock.calls).toEqual([
				[{ default_landing_page: "agents" }],
				[{ default_landing_page: "agents" }],
			]);
		});

		it("does not load or change preferences when Agents is unavailable", async () => {
			vi.spyOn(API, "checkAuthorization").mockResolvedValue({
				...MockPermissions,
				createChat: false,
			});
			const getPreference = vi
				.spyOn(API, "getUserPreferenceSettings")
				.mockReturnValue(createDeferred<UserPreferenceSettings>().promise);
			const update = vi.spyOn(API, "updateUserPreferenceSettings");
			renderWithAuth(<AccountPage />);

			await screen.findByRole("textbox", { name: "Username" });
			expect(getPreference).not.toHaveBeenCalled();
			expect(update).not.toHaveBeenCalled();
		});

		it("defaults to Workspaces after a failed load and allows saving Agents", async () => {
			const user = userEvent.setup();
			const updated: UserPreferenceSettings = {
				...MockUserPreferenceSettings,
				default_landing_page: "agents",
			};
			const initialLoad = createDeferred<UserPreferenceSettings>();
			const getPreference = vi
				.spyOn(API, "getUserPreferenceSettings")
				.mockReturnValueOnce(initialLoad.promise)
				.mockResolvedValue(updated);
			const update = vi
				.spyOn(API, "updateUserPreferenceSettings")
				.mockResolvedValue(updated);
			const { queryClient } = renderWithAuth(<AccountPage />);
			await screen.findByText("Loading default landing page");
			const status = screen.getByRole("status");
			expect(status).toHaveTextContent("Loading default landing page");
			await act(async () =>
				initialLoad.reject(new Error("Preferences unavailable")),
			);
			await waitFor(() => {
				expect(queryClient.getQueryState(preferenceSettingsKey)?.status).toBe(
					"error",
				);
			});
			expect(status.textContent).toBe("");
			const trigger = screen.getByRole("combobox", {
				name: "Default landing page",
			});
			await user.click(trigger);
			await user.click(
				await screen.findByRole("option", { name: "Workspaces" }),
			);
			expect(update).not.toHaveBeenCalled();
			expect(getPreference).toHaveBeenCalledTimes(1);

			await user.click(trigger);
			await user.click(await screen.findByRole("option", { name: "Agents" }));
			await waitFor(() => {
				expect(queryClient.getQueryData(preferenceSettingsKey)).toEqual(
					updated,
				);
			});
			expect(update).toHaveBeenCalledExactlyOnceWith({
				default_landing_page: "agents",
			});
		});
	});

	describe("when it is a success", () => {
		it("shows the success message", async () => {
			vi.spyOn(API, "updateProfile").mockImplementationOnce((userId, data) =>
				Promise.resolve({
					id: userId,
					email: "user@coder.com",
					created_at: new Date().toISOString(),
					updated_at: new Date().toISOString(),
					status: "active",
					organization_ids: ["123"],
					roles: [],
					last_seen_at: new Date().toISOString(),
					login_type: "password",
					has_ai_seat: false,
					theme_preference: "",
					...data,
				}),
			);
			renderWithAuth(<AccountPage />);
			await fillAndSubmitForm();

			const successMessage = await screen.findByText(
				"Profile updated successfully.",
			);
			expect(successMessage).toBeDefined();
			expect(API.updateProfile).toBeCalledTimes(1);
			expect(API.updateProfile).toBeCalledWith("me", newData);
		});
	});

	describe("when the username is already taken", () => {
		it("shows an error", async () => {
			vi.spyOn(API, "updateProfile").mockRejectedValueOnce(
				mockApiError({
					message: "Invalid profile",
					validations: [
						{ detail: "Username is already in use", field: "username" },
					],
				}),
			);

			renderWithAuth(<AccountPage />);
			await fillAndSubmitForm();

			const errorMessages = await screen.findAllByText(
				"Username is already in use",
			);
			expect(errorMessages.length).toBeGreaterThanOrEqual(2);
			expect(API.updateProfile).toBeCalledTimes(1);
			expect(API.updateProfile).toBeCalledWith("me", newData);
		});
	});

	describe("when it is an unknown error", () => {
		it("shows a generic error message", async () => {
			vi.spyOn(API, "updateProfile").mockRejectedValueOnce({
				data: "unknown error",
			});

			renderWithAuth(<AccountPage />);
			await fillAndSubmitForm();

			const errorMessage = await screen.findByText("Something went wrong.");
			expect(errorMessage).toBeDefined();
			expect(API.updateProfile).toBeCalledTimes(1);
			expect(API.updateProfile).toBeCalledWith("me", newData);
		});
	});
});
