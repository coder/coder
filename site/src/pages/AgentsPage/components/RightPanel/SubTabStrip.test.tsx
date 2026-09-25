import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { type SubTab, SubTabStrip } from "./SubTabStrip";

const renderStrip = (
	overrides: Partial<Parameters<typeof SubTabStrip>[0]> = {},
) => {
	const onActiveTabChange = vi.fn();
	const onCloseSecond = vi.fn();
	const tabs: SubTab[] = [
		{ id: "one", label: "Terminal 1" },
		{ id: "two", label: "Terminal 2", onClose: onCloseSecond },
		{ id: "three", label: "Claude Code" },
	];
	renderComponent(
		<SubTabStrip
			label="Terminals"
			idPrefix="strip"
			tabs={tabs}
			activeTabId="one"
			onActiveTabChange={onActiveTabChange}
			{...overrides}
		/>,
	);
	return { onActiveTabChange, onCloseSecond };
};

describe("SubTabStrip", () => {
	it("selects a chip on click", async () => {
		const user = userEvent.setup();
		const { onActiveTabChange } = renderStrip();

		await user.click(screen.getByRole("tab", { name: "Terminal 2" }));

		expect(onActiveTabChange).toHaveBeenCalledWith("two");
	});

	it("closes a chip without selecting it", async () => {
		const user = userEvent.setup();
		const { onActiveTabChange, onCloseSecond } = renderStrip();

		await user.click(screen.getByRole("button", { name: "Close Terminal 2" }));

		expect(onCloseSecond).toHaveBeenCalledTimes(1);
		expect(onActiveTabChange).not.toHaveBeenCalled();
	});

	it("moves the selection with arrow keys and wraps around", async () => {
		const user = userEvent.setup();
		const { onActiveTabChange } = renderStrip({ activeTabId: "three" });

		screen.getByRole("tab", { name: "Claude Code" }).focus();
		await user.keyboard("{ArrowRight}");
		expect(onActiveTabChange).toHaveBeenLastCalledWith("one");

		await user.keyboard("{ArrowLeft}");
		expect(onActiveTabChange).toHaveBeenLastCalledWith("two");
	});

	it("jumps to the first and last chip with Home and End", async () => {
		const user = userEvent.setup();
		const { onActiveTabChange } = renderStrip({ activeTabId: "two" });

		screen.getByRole("tab", { name: "Terminal 2" }).focus();
		await user.keyboard("{End}");
		expect(onActiveTabChange).toHaveBeenLastCalledWith("three");

		await user.keyboard("{Home}");
		expect(onActiveTabChange).toHaveBeenLastCalledWith("one");
	});
});
