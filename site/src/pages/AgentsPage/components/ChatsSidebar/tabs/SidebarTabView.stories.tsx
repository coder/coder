import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { SidebarTab } from "./SidebarTabView";
import { SidebarTabView } from "./SidebarTabView";

const makePanelContent = (label: string) => (
	<div className="flex h-full items-center justify-center p-6 text-sm text-content-secondary">
		Content for {label}
	</div>
);

const makeTab = (id: string, label: string, badge?: number): SidebarTab => ({
	id,
	label,
	badge,
	content: makePanelContent(label),
});

const summaryTab = makeTab("summary", "Summary");
const gitTab = makeTab("git", "Git", 2);
const terminalTab = makeTab("terminal", "Terminal", 3);
const browserTab = makeTab("browser", "Browser");
const desktopTab = makeTab("desktop", "Desktop");
const workspaceTab = makeTab("workspace", "Workspace", 2);
const debugTab = makeTab("debug", "Debug");

const allTabs = [
	summaryTab,
	gitTab,
	terminalTab,
	browserTab,
	desktopTab,
	workspaceTab,
	debugTab,
];

const meta: Meta<typeof SidebarTabView> = {
	title: "pages/AgentsPage/SidebarTabView",
	component: SidebarTabView,
	args: {
		tabs: [summaryTab, gitTab],
		effectiveTabId: "git",
		onActiveTabChange: fn(),
		isExpanded: false,
		onToggleExpanded: fn(),
		onClose: fn(),
	},
	decorators: [
		(Story) => (
			<div style={{ height: 500, width: 480 }}>
				<Story />
			</div>
		),
	],
};
export default meta;
type Story = StoryObj<typeof SidebarTabView>;

export const GitWithBadge: Story = {};

export const GitNoBadge: Story = {
	args: {
		tabs: [summaryTab, { ...gitTab, badge: undefined }],
	},
};

export const AllTabs: Story = {
	args: {
		tabs: allTabs,
		effectiveTabId: "terminal",
	},
};

export const EmptyState: Story = {
	args: {
		tabs: [],
	},
};

export const ExpandedWithTitle: Story = {
	args: {
		tabs: allTabs,
		isExpanded: true,
		chatTitle: "Fix authentication bug",
	},
	decorators: [
		(Story) => (
			<div style={{ height: 500, width: 900 }}>
				<Story />
			</div>
		),
	],
};

/** Every tab at the minimum panel width, which overflows into the scroll chevrons. */
export const NarrowPanelOverflow: Story = {
	args: {
		tabs: allTabs,
		effectiveTabId: "workspace",
	},
	decorators: [
		(Story) => (
			<div style={{ height: 500, width: 360 }}>
				<Story />
			</div>
		),
	],
};
