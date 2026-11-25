import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { SidebarTab, SidebarTabView } from "./SidebarTabView";

const OverviewHarness: React.FC<{ isMounted: boolean }> = ({ isMounted }) => {
	const [focusRequestId, setFocusRequestId] = useState<number>();
	return (
		<>
			<button
				type="button"
				onClick={() => setFocusRequestId((id) => (id ?? 0) + 1)}
			>
				Open Summary
			</button>
			{isMounted && (
				<SidebarTabView
					activeTabId="summary"
					onActiveTabChange={vi.fn()}
					isPanelOpen
					isExpanded={false}
					onToggleExpanded={vi.fn()}
					focusRequestId={focusRequestId}
					onFocusRequestApplied={(id) =>
						setFocusRequestId((current) =>
							current === id ? undefined : current,
						)
					}
				>
					<SidebarTab id="summary" label="Summary">
						{null}
					</SidebarTab>
				</SidebarTabView>
			)}
		</>
	);
};

describe("SidebarTabView focus requests", () => {
	it("moves focus once per request, not again after a remount", async () => {
		const user = userEvent.setup();
		const { rerender } = renderComponent(<OverviewHarness isMounted />);
		const openSummary = screen.getByRole("button", { name: "Open Summary" });

		await user.click(openSummary);
		expect(screen.getByRole("tab", { name: "Summary" })).toHaveFocus();

		act(() => openSummary.focus());
		rerender(<OverviewHarness isMounted={false} />);
		rerender(<OverviewHarness isMounted />);

		expect(openSummary).toHaveFocus();
	});
});

describe("SidebarTabView tab scrolling", () => {
	// jsdom's :focus-visible inside a focus handler depends on earlier focus
	// changes in the file, so the CloseableTabLabelFocusedWithKeyboard story
	// covers keyboard focus.
	it("does not scroll a tab into view on pointer focus", async () => {
		const user = userEvent.setup();
		const onActiveTabChange = vi.fn();
		renderComponent(
			<SidebarTabView
				activeTabId="git"
				onActiveTabChange={onActiveTabChange}
				isPanelOpen
				isExpanded={false}
				onToggleExpanded={vi.fn()}
			>
				<SidebarTab id="git" label="Git">
					{null}
				</SidebarTab>
				<SidebarTab id="terminal" label="Terminal" onClose={vi.fn()}>
					{null}
				</SidebarTab>
			</SidebarTabView>,
		);

		await user.click(screen.getByRole("tab", { name: "Terminal" }));

		expect(onActiveTabChange).toHaveBeenCalledWith("terminal");
		expect(HTMLElement.prototype.scrollIntoView).not.toHaveBeenCalled();
	});
});
