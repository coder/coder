import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { RightPanelAddTabControl } from "./RightPanelAddTabControl";

const terminalPanel = vi.hoisted(() => ({ loads: 0 }));

vi.mock("../TerminalPanel", () => {
	terminalPanel.loads++;
	return { default: () => null };
});

const props = {
	workspace: MockWorkspace,
	agent: MockWorkspaceAgent,
	host: "",
	supportedSingletonTabs: [],
	visibleSingletonTabs: [],
	onToggleSingletonTab: () => {},
	onNewTerminal: () => {},
	onOpenWorkspaceApp: () => {},
	onOpenCommandApp: () => {},
	onOpenPort: () => {},
};

describe("RightPanelAddTabControl", () => {
	it("starts loading the terminal panel on hover only when a terminal can open", async () => {
		const user = userEvent.setup();
		const { rerender } = render(
			<RightPanelAddTabControl {...props} isRunning={false} />,
		);
		const addPanel = screen.getByRole("button", { name: "Add panel" });
		await user.hover(addPanel);
		await vi.dynamicImportSettled();
		expect(terminalPanel.loads).toBe(0);

		await user.unhover(addPanel);
		rerender(<RightPanelAddTabControl {...props} isRunning />);
		await user.hover(addPanel);
		await vi.dynamicImportSettled();
		expect(terminalPanel.loads).toBe(1);
	});
});
