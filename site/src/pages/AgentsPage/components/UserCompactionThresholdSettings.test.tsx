import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, Link, Outlet } from "react-router";
import { describe, expect, it, vi } from "vitest";
import { MockChatModel } from "#/testHelpers/chatModels";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { UserCompactionThresholdSettings } from "./UserCompactionThresholdSettings";

const mockModels = [
	{
		...MockChatModel,
		id: "model-1",
		organization_id: MockDefaultOrganization.id,
		display_name: "First model",
	},
	{
		...MockChatModel,
		id: "model-2",
		organization_id: MockDefaultOrganization.id,
		display_name: "Second model",
	},
];

const setup = (
	props: Partial<
		React.ComponentProps<typeof UserCompactionThresholdSettings>
	> = {},
) => {
	const onSaveThreshold = vi.fn(async () => undefined);
	const onResetThreshold = vi.fn(async () => undefined);
	const router = createMemoryRouter(
		[
			{
				element: (
					<>
						<Link to="/general">General settings</Link>
						<Outlet />
					</>
				),
				children: [
					{
						path: "/compaction",
						element: (
							<UserCompactionThresholdSettings
								models={mockModels}
								providerInfoByID={new Map()}
								organizations={[MockDefaultOrganization]}
								compactionTriggersByOrganizationID={new Map()}
								compactionTriggerLoadErrors={[]}
								isLoadingModels={false}
								thresholds={[]}
								isThresholdsLoading={false}
								thresholdsError={undefined}
								onSaveThreshold={onSaveThreshold}
								onResetThreshold={onResetThreshold}
								{...props}
							/>
						),
					},
					{ path: "/general", element: <h1>General settings</h1> },
				],
			},
		],
		{ initialEntries: ["/compaction"] },
	);
	renderWithRouter(router);
	return { router, onSaveThreshold, onResetThreshold, user: userEvent.setup() };
};

const rowFor = (name: string) =>
	within(screen.getByRole("row", { name: new RegExp(name) }));

const unloadIsBlocked = () => {
	const event = new Event("beforeunload", { cancelable: true });
	window.dispatchEvent(event);
	return event.defaultPrevented;
};

describe("UserCompactionThresholdSettings", () => {
	it("saves only the submitted row, prevents duplicate saves, and retains failed drafts for retry", async () => {
		const pendingSave = createDeferred<unknown>();
		const onSaveThreshold = vi
			.fn()
			.mockReturnValueOnce(pendingSave.promise)
			.mockRejectedValueOnce(new Error("Network error"))
			.mockResolvedValue(undefined);
		const { user, router } = setup({ onSaveThreshold });
		const first = rowFor("First model");
		const second = rowFor("Second model");
		await user.type(first.getByRole("textbox"), "90");
		await user.type(second.getByRole("textbox"), "55");
		await user.click(first.getByRole("button", { name: /^Save/ }));
		expect(document.activeElement).toBe(first.getByRole("textbox"));
		await user.click(first.getByRole("button", { name: /^Save/ }));
		expect(onSaveThreshold).toHaveBeenCalledExactlyOnceWith("model-1", 90);
		expect(unloadIsBlocked()).toBe(true);

		// A different row remains editable while the first request is pending.
		await user.click(second.getByRole("button", { name: /^Save/ }));
		expect(onSaveThreshold).toHaveBeenNthCalledWith(2, "model-2", 55);
		await act(async () => pendingSave.resolve(undefined));
		expect(document.activeElement).toBe(second.getByRole("textbox"));
		await user.click(second.getByRole("button", { name: /^Save/ }));
		expect(onSaveThreshold).toHaveBeenNthCalledWith(3, "model-2", 55);
		await waitFor(() => expect(unloadIsBlocked()).toBe(false));
		await user.click(screen.getByRole("link", { name: "General settings" }));
		expect(router.state.location.pathname).toBe("/general");
	});

	it("cancels only its own row and submits the remaining draft with Enter", async () => {
		const { user, onSaveThreshold } = setup();
		const first = rowFor("First model");
		const second = rowFor("Second model");
		await user.type(first.getByRole("textbox"), "90");
		await user.type(second.getByRole("textbox"), "55");
		await user.click(first.getByRole("button", { name: /^Cancel/ }));
		expect(document.activeElement).toBe(first.getByRole("textbox"));
		expect(unloadIsBlocked()).toBe(true);
		await user.type(second.getByRole("textbox"), "{Enter}");
		expect(onSaveThreshold).toHaveBeenCalledExactlyOnceWith("model-2", 55);
		await waitFor(() => expect(unloadIsBlocked()).toBe(false));
	});

	it.each(["90", "150"])(
		"requires confirmation to leave with a %s draft and preserves it when staying",
		async (draft) => {
			const { user, router, onSaveThreshold } = setup();
			const first = rowFor("First model");
			await user.type(first.getByRole("textbox"), draft);
			if (draft === "150") {
				await user.click(first.getByRole("button", { name: /^Save/ }));
				await user.type(first.getByRole("textbox"), "{Enter}");
				expect(onSaveThreshold).not.toHaveBeenCalled();
			}
			expect(unloadIsBlocked()).toBe(true);
			await user.click(screen.getByRole("link", { name: "General settings" }));
			expect(router.state.location.pathname).toBe("/compaction");
			await user.click(await screen.findByRole("button", { name: "Stay" }));
			await waitFor(() =>
				expect(document.activeElement).toBe(
					screen.getByRole("link", { name: "General settings" }),
				),
			);
			expect(router.state.location.pathname).toBe("/compaction");
			await user.keyboard("{Enter}");
			await user.click(await screen.findByRole("button", { name: "Leave" }));
			expect(router.state.location.pathname).toBe("/general");
			await waitFor(() =>
				expect(document.activeElement).toBe(
					screen.getByRole("link", { name: "General settings" }),
				),
			);
			expect(unloadIsBlocked()).toBe(false);
		},
	);

	it.each(["cancel", "revert", "reset"])(
		"stops warning after %s removes the last change",
		async (action) => {
			const { user, router, onResetThreshold } = setup({
				thresholds: [{ model_config_id: "model-1", threshold_percent: 90 }],
			});
			const first = rowFor("First model");
			const input = first.getByRole("textbox");
			expect(unloadIsBlocked()).toBe(false);
			await user.clear(input);
			expect(unloadIsBlocked()).toBe(true);
			if (action === "cancel") {
				await user.click(first.getByRole("button", { name: /^Cancel/ }));
			} else if (action === "revert") {
				await user.type(input, "90");
			} else {
				await user.click(first.getByRole("button", { name: /Reset/ }));
				expect(onResetThreshold).toHaveBeenCalledExactlyOnceWith("model-1");
			}
			await waitFor(() => expect(unloadIsBlocked()).toBe(false));
			await user.click(screen.getByRole("link", { name: "General settings" }));
			expect(router.state.location.pathname).toBe("/general");
		},
	);

	it("protects hidden organization drafts while saving another row", async () => {
		const { user, router, onSaveThreshold } = setup({
			models: [
				mockModels[0],
				{ ...mockModels[1], organization_id: MockOrganization2.id },
			],
			organizations: [MockDefaultOrganization, MockOrganization2],
		});
		await user.type(rowFor("First model").getByRole("textbox"), "95");
		await user.click(screen.getByRole("combobox"));
		await user.click(
			screen.getByRole("option", {
				name: new RegExp(`${MockOrganization2.display_name}$`),
			}),
		);
		await user.type(rowFor("Second model").getByRole("textbox"), "50");
		await user.click(
			rowFor("Second model").getByRole("button", { name: /^Save/ }),
		);
		expect(onSaveThreshold).toHaveBeenCalledExactlyOnceWith("model-2", 50);
		expect(unloadIsBlocked()).toBe(true);
		await user.click(screen.getByRole("link", { name: "General settings" }));
		expect(router.state.location.pathname).toBe("/compaction");
		await user.click(await screen.findByRole("button", { name: "Stay" }));
		await user.click(screen.getByRole("combobox"));
		await user.click(
			screen.getByRole("option", {
				name: new RegExp(`${MockDefaultOrganization.display_name}$`),
			}),
		);
		await user.click(
			rowFor("First model").getByRole("button", { name: /^Save/ }),
		);
		expect(onSaveThreshold).toHaveBeenNthCalledWith(2, "model-1", 95);
		await waitFor(() => expect(unloadIsBlocked()).toBe(false));
	});
});
