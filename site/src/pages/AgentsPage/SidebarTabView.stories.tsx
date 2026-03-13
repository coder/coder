import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { SidebarTab } from "./SidebarTabView";
import { SidebarTabView } from "./SidebarTabView";

const makePanelContent = (label: string) => (
	<div className="flex h-full items-center justify-center p-6 text-sm text-content-secondary">
		Content for {label}
	</div>
);

const makeBadge = (additions: number, deletions: number) => (
	<span className="inline-flex h-full items-center self-stretch overflow-hidden font-mono text-xs font-medium">
		{additions > 0 && (
			<span className="flex h-full items-center bg-surface-git-added px-1.5 text-git-added-bright">
				+{additions}
			</span>
		)}
		{deletions > 0 && (
			<span className="flex h-full items-center bg-surface-git-deleted px-1.5 text-git-deleted-bright">
				&minus;{deletions}
			</span>
		)}
	</span>
);

const gitTab: SidebarTab = {
	id: "git",
	label: "Git",
	badge: makeBadge(42, 7),
	content: makePanelContent("Git"),
};

const meta: Meta<typeof SidebarTabView> = {
	title: "pages/AgentsPage/SidebarTabView",
	component: SidebarTabView,
	args: {
		tabs: [gitTab],
		isExpanded: false,
		onToggleExpanded: fn(),
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
		tabs: [{ ...gitTab, badge: undefined }],
	},
};

export const MultipleTabs: Story = {
	args: {
		tabs: [
			gitTab,
			{ id: "preview", label: "Preview", content: makePanelContent("Preview") },
		],
	},
};

export const EmptyState: Story = {
	args: {
		tabs: [],
	},
};

export const ExpandedWithTitle: Story = {
	args: {
		tabs: [gitTab],
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

export const NarrowPanel: Story = {
	args: {
		tabs: [gitTab],
	},
	decorators: [
		(Story) => (
			<div style={{ height: 500, width: 360 }}>
				<Story />
			</div>
		),
	],
};
