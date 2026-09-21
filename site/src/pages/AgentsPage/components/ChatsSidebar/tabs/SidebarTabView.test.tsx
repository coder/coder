import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type FC, useEffect } from "react";
import { describe, expect, it, vi } from "vitest";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { type SidebarTab, SidebarTabView } from "./SidebarTabView";

const LifecycleSpy: FC<{ onMount: () => void; onUnmount: () => void }> = ({
	onMount,
	onUnmount,
}) => {
	useEffect(() => {
		onMount();
		return onUnmount;
	}, [onMount, onUnmount]);
	return null;
};

const renderView = (
	overrides: Partial<Parameters<typeof SidebarTabView>[0]> = {},
) => {
	const onActiveTabChange = vi.fn();
	const onToggleExpanded = vi.fn();
	const onClose = vi.fn();
	const tabs: SidebarTab[] = [
		{ id: "summary", label: "Summary", content: <div>Summary body</div> },
		{ id: "git", label: "Git", badge: 2, content: <div>Git body</div> },
	];
	const view = renderComponent(
		<SidebarTabView
			tabs={tabs}
			effectiveTabId="summary"
			onActiveTabChange={onActiveTabChange}
			isExpanded={false}
			onToggleExpanded={onToggleExpanded}
			onClose={onClose}
			{...overrides}
		/>,
	);
	return { ...view, onActiveTabChange, onToggleExpanded, onClose };
};

describe("SidebarTabView", () => {
	it("reports the clicked tab", async () => {
		const user = userEvent.setup();
		const { onActiveTabChange } = renderView();

		await user.click(screen.getByRole("tab", { name: "Git 2" }));

		expect(onActiveTabChange).toHaveBeenCalledWith("git");
	});

	it("moves between tabs with the keyboard", async () => {
		const user = userEvent.setup();
		const { onActiveTabChange } = renderView();

		screen.getByRole("tab", { name: "Summary" }).focus();
		await user.keyboard("{ArrowRight}");

		expect(onActiveTabChange).toHaveBeenCalledWith("git");
	});

	it("wires the header actions", async () => {
		const user = userEvent.setup();
		const { onToggleExpanded, onClose } = renderView();

		await user.click(screen.getByRole("button", { name: "Expand panel" }));
		await user.click(screen.getByRole("button", { name: "Close panel" }));

		expect(onToggleExpanded).toHaveBeenCalledTimes(1);
		expect(onClose).toHaveBeenCalledTimes(1);
	});

	it("mounts a tab's content on first selection and keeps it mounted", () => {
		const mountSpy = vi.fn();
		const unmountSpy = vi.fn();
		const tabs: SidebarTab[] = [
			{ id: "summary", label: "Summary", content: <div>Summary body</div> },
			{
				id: "browser",
				label: "Browser",
				content: <LifecycleSpy onMount={mountSpy} onUnmount={unmountSpy} />,
			},
		];
		const { rerender } = renderView({ tabs, effectiveTabId: "summary" });
		expect(mountSpy).not.toHaveBeenCalled();

		rerender(
			<SidebarTabView
				tabs={tabs}
				effectiveTabId="browser"
				onActiveTabChange={vi.fn()}
				isExpanded={false}
				onToggleExpanded={vi.fn()}
			/>,
		);
		expect(mountSpy).toHaveBeenCalledTimes(1);

		rerender(
			<SidebarTabView
				tabs={tabs}
				effectiveTabId="summary"
				onActiveTabChange={vi.fn()}
				isExpanded={false}
				onToggleExpanded={vi.fn()}
			/>,
		);
		expect(unmountSpy).not.toHaveBeenCalled();
	});
});
