import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { userNotificationPreferencesKey } from "#/api/queries/notifications";
import type { NotificationPreference } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockCustomNotificationTemplates,
	MockNotificationMethodsResponse,
	MockNotificationPreferences,
	MockSystemNotificationTemplates,
	MockUserOwner,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import NotificationsPage from "./NotificationsPage";

const deletedTemplate = MockSystemNotificationTemplates.find(
	(template) => template.name === "Workspace Deleted",
);
const dormantTemplate = MockSystemNotificationTemplates.find(
	(template) => template.name === "Workspace Marked as Dormant",
);
if (!deletedTemplate || !dormantTemplate) {
	throw new Error("Workspace notification fixtures are missing.");
}
const deletedId = deletedTemplate.id;
const dormantId = dormantTemplate.id;
const preferencesKey = userNotificationPreferencesKey(MockUserOwner.id);

beforeEach(() => {
	vi.spyOn(API, "getUserNotificationPreferences").mockResolvedValue(
		MockNotificationPreferences,
	);
	vi.spyOn(API, "getSystemNotificationTemplates").mockResolvedValue(
		MockSystemNotificationTemplates,
	);
	vi.spyOn(API, "getCustomNotificationTemplates").mockResolvedValue(
		MockCustomNotificationTemplates,
	);
	vi.spyOn(API, "getNotificationDispatchMethods").mockResolvedValue(
		MockNotificationMethodsResponse,
	);
});

afterEach(() => {
	vi.restoreAllMocks();
});

describe("NotificationsPage", () => {
	it.each(["toggle", "unsubscribe link"])(
		"keeps rapid notification changes without resending unrelated preferences (%s)",
		async (firstAction) => {
			const firstSave = createDeferred<NotificationPreference[]>();
			const secondSave = createDeferred<NotificationPreference[]>();
			const update = vi
				.spyOn(API, "putUserNotificationPreferences")
				.mockReturnValueOnce(firstSave.promise)
				.mockReturnValueOnce(secondSave.promise);
			const savedPreferences = MockNotificationPreferences.map((preference) =>
				preference.id === deletedId || preference.id === dormantId
					? { ...preference, disabled: true }
					: preference,
			);
			const { queryClient, router } = renderWithAuth(<NotificationsPage />);
			const user = userEvent.setup();
			const deletedSwitch = await screen.findByRole("switch", {
				name: "Workspace Deleted",
			});

			if (firstAction === "toggle") {
				await user.click(deletedSwitch);
			} else {
				await act(async () => router.navigate(`/?disabled=${deletedId}`));
			}
			await user.click(
				screen.getByRole("switch", { name: "Workspace Marked as Dormant" }),
			);

			expect(update).toHaveBeenNthCalledWith(1, MockUserOwner.id, {
				template_disabled_map: { [deletedId]: true },
			});
			expect(update).toHaveBeenCalledTimes(1);

			await act(async () => {
				firstSave.resolve(
					MockNotificationPreferences.map((preference) =>
						preference.id === deletedId
							? { ...preference, disabled: true }
							: preference,
					),
				);
			});
			await waitFor(() => expect(update).toHaveBeenCalledTimes(2));
			expect(update).toHaveBeenNthCalledWith(2, MockUserOwner.id, {
				template_disabled_map: { [dormantId]: true },
			});
			expect(API.getUserNotificationPreferences).toHaveBeenCalledTimes(1);

			vi.mocked(API.getUserNotificationPreferences).mockResolvedValue(
				savedPreferences,
			);
			await act(async () => secondSave.resolve(savedPreferences));
			await waitFor(() => expect(queryClient.isMutating()).toBe(0));
			expect(API.getUserNotificationPreferences).toHaveBeenCalledTimes(2);
			expect(queryClient.getQueryData(preferencesKey)).toEqual(
				savedPreferences,
			);
		},
	);

	it("orders group and repeated row toggles without losing the latest choice", async () => {
		const firstSave = createDeferred<NotificationPreference[]>();
		const secondSave = createDeferred<NotificationPreference[]>();
		const thirdSave = createDeferred<NotificationPreference[]>();
		const update = vi
			.spyOn(API, "putUserNotificationPreferences")
			.mockReturnValueOnce(firstSave.promise)
			.mockReturnValueOnce(secondSave.promise)
			.mockReturnValueOnce(thirdSave.promise);
		const group = MockSystemNotificationTemplates.filter(
			(template) => template.group === "Template Events",
		);
		const groupPreferences = group.map(({ id }) => ({
			id,
			disabled: true,
			updated_at: MockNotificationPreferences[0].updated_at,
		}));
		const savedPreferences = [
			...MockNotificationPreferences,
			...group.map(({ id, name }) => ({
				id,
				disabled: name !== "Template Updated",
				updated_at: MockNotificationPreferences[0].updated_at,
			})),
		];
		const { queryClient } = renderWithAuth(<NotificationsPage />);
		const user = userEvent.setup();

		await user.click(
			await screen.findByRole("switch", { name: "Template Events" }),
		);
		await user.click(screen.getByRole("switch", { name: "Template Updated" }));
		expect(update).toHaveBeenNthCalledWith(1, MockUserOwner.id, {
			template_disabled_map: Object.fromEntries(
				group.map(({ id }) => [id, true]),
			),
		});
		expect(update).toHaveBeenCalledTimes(1);

		await act(async () => {
			firstSave.resolve([...MockNotificationPreferences, ...groupPreferences]);
		});
		await waitFor(() => expect(update).toHaveBeenCalledTimes(2));
		expect(update).toHaveBeenNthCalledWith(2, MockUserOwner.id, {
			template_disabled_map: Object.fromEntries(
				group
					.filter(({ name }) => name === "Template Updated")
					.map(({ id }) => [id, false]),
			),
		});

		await user.click(screen.getByRole("switch", { name: "Template Updated" }));
		expect(update).toHaveBeenCalledTimes(2);
		await act(async () => secondSave.resolve(savedPreferences));
		await waitFor(() => expect(update).toHaveBeenCalledTimes(3));
		expect(update).toHaveBeenNthCalledWith(3, MockUserOwner.id, {
			template_disabled_map: Object.fromEntries(
				group
					.filter(({ name }) => name === "Template Updated")
					.map(({ id }) => [id, true]),
			),
		});
		const finalPreferences = [
			...MockNotificationPreferences,
			...groupPreferences,
		];
		vi.mocked(API.getUserNotificationPreferences).mockResolvedValue(
			finalPreferences,
		);
		await act(async () => thirdSave.resolve(finalPreferences));
		await waitFor(() => expect(queryClient.isMutating()).toBe(0));
		expect(queryClient.getQueryData(preferencesKey)).toEqual(finalPreferences);
	});

	it.each(["before", "after"])(
		"preserves pending choices when a preference read starts %s a toggle",
		async (readTiming) => {
			const pendingRead = createDeferred<NotificationPreference[]>();
			const pendingSave = createDeferred<NotificationPreference[]>();
			const update = vi
				.spyOn(API, "putUserNotificationPreferences")
				.mockReturnValueOnce(pendingSave.promise)
				.mockResolvedValueOnce(MockNotificationPreferences);
			const savedPreferences = MockNotificationPreferences.map((preference) =>
				preference.id === deletedId
					? { ...preference, disabled: true }
					: preference,
			);
			const { queryClient } = renderWithAuth(<NotificationsPage />);
			const user = userEvent.setup();
			const deletedSwitch = await screen.findByRole("switch", {
				name: "Workspace Deleted",
			});

			if (readTiming === "after") {
				await user.click(deletedSwitch);
			}
			vi.mocked(API.getUserNotificationPreferences).mockReturnValueOnce(
				pendingRead.promise,
			);
			const refetch = queryClient.refetchQueries({ queryKey: preferencesKey });
			await waitFor(() =>
				expect(API.getUserNotificationPreferences).toHaveBeenCalledTimes(2),
			);
			if (readTiming === "before") {
				await user.click(deletedSwitch);
			}
			await act(async () => {
				pendingRead.resolve(MockNotificationPreferences);
				await refetch;
			});
			await user.click(deletedSwitch);
			await act(async () => pendingSave.resolve(savedPreferences));
			await waitFor(() => expect(update).toHaveBeenCalledTimes(2));
			expect(update).toHaveBeenNthCalledWith(2, MockUserOwner.id, {
				template_disabled_map: { [deletedId]: false },
			});
			await waitFor(() => expect(queryClient.isMutating()).toBe(0));
			expect(queryClient.getQueryData(preferencesKey)).toEqual(
				MockNotificationPreferences,
			);
		},
	);

	it.each([
		{ failure: "first", refetchFails: false },
		{ failure: "second", refetchFails: false },
		{ failure: "both", refetchFails: false },
		{ failure: "first", refetchFails: true },
		{ failure: "both", refetchFails: true },
	])(
		"recovers from failed requests: $failure, refetch fails: $refetchFails",
		async ({ failure, refetchFails }) => {
			const firstSave = createDeferred<NotificationPreference[]>();
			const secondSave = createDeferred<NotificationPreference[]>();
			const update = vi
				.spyOn(API, "putUserNotificationPreferences")
				.mockReturnValueOnce(firstSave.promise)
				.mockReturnValueOnce(secondSave.promise);
			const showError = vi.spyOn(toast, "error");
			const savedPreferences = MockNotificationPreferences.map((preference) => {
				if (preference.id === deletedId) {
					return { ...preference, disabled: failure === "second" };
				}
				if (preference.id === dormantId) {
					return { ...preference, disabled: failure === "first" };
				}
				return preference;
			});
			const { queryClient } = renderWithAuth(<NotificationsPage />);
			const user = userEvent.setup();
			await user.click(
				await screen.findByRole("switch", { name: "Workspace Deleted" }),
			);
			await user.click(
				screen.getByRole("switch", { name: "Workspace Marked as Dormant" }),
			);

			await act(async () => {
				if (failure === "second") {
					firstSave.resolve(savedPreferences);
				} else {
					firstSave.reject(new Error("Unable to save preferences"));
				}
			});
			await waitFor(() => expect(update).toHaveBeenCalledTimes(2));
			expect(API.getUserNotificationPreferences).toHaveBeenCalledTimes(1);

			if (refetchFails) {
				vi.mocked(API.getUserNotificationPreferences).mockRejectedValue(
					new Error("Unable to load preferences"),
				);
			} else {
				vi.mocked(API.getUserNotificationPreferences).mockResolvedValue(
					savedPreferences,
				);
			}
			await act(async () => {
				if (failure === "first") {
					secondSave.resolve(savedPreferences);
				} else {
					secondSave.reject(new Error("Unable to save preferences"));
				}
			});
			await waitFor(() => expect(queryClient.isMutating()).toBe(0));
			expect(API.getUserNotificationPreferences).toHaveBeenCalledTimes(2);
			expect(queryClient.getQueryData(preferencesKey)).toEqual(
				savedPreferences,
			);
			expect(showError).toHaveBeenCalledTimes(failure === "both" ? 2 : 1);

			update.mockResolvedValueOnce(savedPreferences);
			await user.click(
				screen.getByRole("switch", { name: "Workspace Deleted" }),
			);
			await waitFor(() => expect(update).toHaveBeenCalledTimes(3));
			expect(update).toHaveBeenNthCalledWith(3, MockUserOwner.id, {
				template_disabled_map: { [deletedId]: failure !== "second" },
			});
			await waitFor(() => expect(queryClient.isMutating()).toBe(0));
		},
	);
});
