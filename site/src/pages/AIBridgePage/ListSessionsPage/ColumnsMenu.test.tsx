import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { ColumnsMenu } from "./ColumnsMenu";
import {
	defaultSessionColumnSettings,
	type SessionColumnSettings,
	sessionColumnViews,
} from "./sessionColumns";

const renderMenu = (settings: SessionColumnSettings) => {
	const onSettingsChange = vi.fn();
	renderComponent(
		<ColumnsMenu
			settings={settings}
			onSettingsChange={onSettingsChange}
			fittingCount={settings.shown.length}
		/>,
	);
	return onSettingsChange;
};

const openMenu = async () => {
	const user = userEvent.setup();
	await user.click(screen.getByRole("button", { name: "Columns" }));
	return user;
};

describe("ColumnsMenu", () => {
	it("applies a view when it is selected", async () => {
		const onSettingsChange = renderMenu(defaultSessionColumnSettings);
		const user = await openMenu();

		await user.click(screen.getByRole("button", { name: "Security" }));

		expect(onSettingsChange).toHaveBeenCalledWith(
			sessionColumnViews[1].settings,
		);
	});

	it("shows a hidden column when its switch is turned on", async () => {
		const onSettingsChange = renderMenu(defaultSessionColumnSettings);
		const user = await openMenu();

		await user.click(screen.getByRole("switch", { name: "Model" }));

		expect(onSettingsChange).toHaveBeenCalledWith(
			expect.objectContaining({
				shown: [...defaultSessionColumnSettings.shown, "model"],
			}),
		);
	});

	it("hides a shown column when its switch is turned off", async () => {
		const onSettingsChange = renderMenu(defaultSessionColumnSettings);
		const user = await openMenu();

		await user.click(screen.getByRole("switch", { name: "User" }));

		expect(onSettingsChange).toHaveBeenCalledWith(
			expect.objectContaining({
				shown: defaultSessionColumnSettings.shown.filter((id) => id !== "user"),
			}),
		);
	});

	it("does not hide the last shown column", async () => {
		const onSettingsChange = renderMenu({
			shown: ["user"],
			notShown: defaultSessionColumnSettings.notShown,
		});
		const user = await openMenu();

		await user.click(screen.getByRole("switch", { name: "User" }));

		expect(onSettingsChange).not.toHaveBeenCalled();
	});

	describe("reordering", () => {
		const rowHeight = 32;
		let restoreDom: () => void;

		// dnd-kit needs animation APIs jsdom lacks, and it picks the drop
		// target from measured rects, so stack the rows in document order.
		const stubDragLayout = () => {
			vi.stubGlobal(
				"IntersectionObserver",
				class {
					observe = vi.fn();
					unobserve = vi.fn();
					disconnect = vi.fn();
				},
			);
			const stubs = [
				[document, { getAnimations: () => [], elementFromPoint: () => null }],
				[
					Element.prototype,
					{
						getAnimations: () => [],
						animate: () => ({ finished: Promise.resolve() }),
					},
				],
			] as const;
			for (const [target, members] of stubs) {
				Object.assign(target, members);
			}
			const handles = screen.getAllByRole("button", { name: /^Reorder / });
			vi.spyOn(Element.prototype, "getBoundingClientRect").mockImplementation(
				function (this: Element) {
					const held = handles.filter((handle) => this.contains(handle));
					if (held.length !== 1) {
						return DOMRect.fromRect({ width: 280, height: 400 });
					}
					const index = handles.indexOf(held[0]);
					return DOMRect.fromRect({
						y: index * rowHeight,
						width: 280,
						height: rowHeight,
					});
				},
			);
			restoreDom = () => {
				vi.unstubAllGlobals();
				for (const [target, members] of stubs) {
					for (const key of Object.keys(members)) {
						Reflect.deleteProperty(target, key);
					}
				}
			};
		};

		afterEach(() => {
			restoreDom?.();
			vi.restoreAllMocks();
		});

		it("moves a shown column with the keyboard", async () => {
			const onSettingsChange = renderMenu(defaultSessionColumnSettings);
			const user = await openMenu();
			stubDragLayout();

			screen.getByRole("button", { name: "Reorder User" }).focus();
			await user.keyboard("[Space]");
			await user.keyboard("[ArrowUp]");
			await user.keyboard("[Space]");

			await waitFor(() =>
				expect(onSettingsChange).toHaveBeenCalledWith(
					expect.objectContaining({
						shown: [
							"user",
							"last_prompt",
							"client",
							"tokens",
							"cost",
							"last_active",
						],
					}),
				),
			);
		});
	});
});
