import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { useChatOverview } from "../../../hooks/useChatOverview";
import { SidebarTabView } from "./SidebarTabView";

const OverviewHarness: React.FC<{ isMounted: boolean }> = ({ isMounted }) => {
	const overview = useChatOverview({ isRightPanelOpen: true });
	return (
		<>
			<button
				type="button"
				onClick={() => overview.recordTarget({ tabId: "summary" })}
			>
				Open Summary
			</button>
			{isMounted && (
				<SidebarTabView
					tabs={[{ id: "summary", label: "Summary", content: null }]}
					effectiveTabId="summary"
					onActiveTabChange={vi.fn()}
					isExpanded={false}
					onToggleExpanded={vi.fn()}
					focusRequestId={overview.focusRequestId}
					onFocusRequestApplied={overview.onFocusRequestApplied}
				/>
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
