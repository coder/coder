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
