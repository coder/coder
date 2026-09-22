import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { type SidebarTab, SidebarTabView } from "./SidebarTabView";

const makeTab = (id: string, label: string): SidebarTab => ({
	id,
	label,
	content: <div>{label} content</div>,
});

describe("SidebarTabView", () => {
	it("switches tabs from the all-tabs menu", async () => {
		const onActiveTabChange = vi.fn();
		render(
			<SidebarTabView
				tabs={[makeTab("summary", "Summary"), makeTab("git", "Git")]}
				effectiveTabId="summary"
				onActiveTabChange={onActiveTabChange}
				isExpanded={false}
				onToggleExpanded={vi.fn()}
			/>,
		);

		await userEvent.click(screen.getByRole("button", { name: "All tabs" }));
		await userEvent.click(
			await screen.findByRole("menuitemradio", { name: "Git" }),
		);

		expect(onActiveTabChange).toHaveBeenCalledWith("git");
	});

	it("closes a tab without activating it", async () => {
		const onActiveTabChange = vi.fn();
		const onClose = vi.fn();
		render(
			<SidebarTabView
				tabs={[
					makeTab("summary", "Summary"),
					{ ...makeTab("terminal", "Terminal"), onClose },
				]}
				effectiveTabId="summary"
				onActiveTabChange={onActiveTabChange}
				isExpanded={false}
				onToggleExpanded={vi.fn()}
			/>,
		);

		await userEvent.click(
			screen.getByRole("button", { name: "Close Terminal tab" }),
		);

		expect(onClose).toHaveBeenCalledTimes(1);
		expect(onActiveTabChange).not.toHaveBeenCalled();
	});
});
