import type { Meta, StoryObj } from "@storybook/react-vite";
import { PlusIcon } from "lucide-react";
import { useState } from "react";
import { expect, fn, userEvent, within } from "storybook/test";
import { Button } from "#/components/Button/Button";
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
		effectiveTabId: "git",
		onActiveTabChange: fn(),
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

export const DesktopHidden: Story = {
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

export const CloseableTabs: Story = {
	render: function CloseableTabs() {
		const [activeTabId, setActiveTabId] = useState("terminal-2");
		const [tabs, setTabs] = useState<SidebarTab[]>([
			gitTab,
			{
				id: "terminal",
				label: "Terminal",
				content: makePanelContent("Terminal"),
			},
			{ id: "debug", label: "Debug", content: makePanelContent("Debug") },
			...Array.from({ length: 8 }, (_, index) => ({
				id: `terminal-${index + 2}`,
				label: `Terminal ${index + 2}`,
				content: makePanelContent(`Terminal ${index + 2}`),
			})),
		]);

		const handleCloseTab = (tabId: string) => {
			const visibleTabIds = tabs.map((tab) => tab.id);
			const remainingTabIds = visibleTabIds.filter((id) => id !== tabId);
			const closedTabIndex = visibleTabIds.indexOf(tabId);
			setTabs(tabs.filter((tab) => tab.id !== tabId));

			if (activeTabId !== tabId) {
				return;
			}
			const nextActiveTabId =
				remainingTabIds[Math.min(closedTabIndex, remainingTabIds.length - 1)];
			if (nextActiveTabId) {
				setActiveTabId(nextActiveTabId);
			}
		};

		return (
			<SidebarTabView
				tabs={tabs.map((tab) => ({
					...tab,
					onClose: tab.id.startsWith("terminal-")
						? () => handleCloseTab(tab.id)
						: undefined,
				}))}
				effectiveTabId={activeTabId}
				onActiveTabChange={setActiveTabId}
				isExpanded={false}
				onToggleExpanded={() => {}}
				addTabControl={
					<Button
						variant="outline"
						size="icon"
						onClick={fn()}
						aria-label="New terminal tab"
						title="New terminal tab"
						className="size-6 bg-surface-primary p-0 text-content-secondary hover:text-content-primary"
					>
						<PlusIcon className="size-3.5" />
					</Button>
				}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);

		expect(
			canvas.queryByRole("button", { name: "Close Git tab" }),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Close Terminal tab" }),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Close Debug tab" }),
		).not.toBeInTheDocument();

		expect(canvas.getByRole("tab", { name: "Terminal 2" })).toHaveAttribute(
			"aria-selected",
			"true",
		);

		await user.click(
			canvas.getByRole("button", { name: "Close Terminal 3 tab" }),
		);

		expect(
			canvas.queryByRole("tab", { name: "Terminal 3" }),
		).not.toBeInTheDocument();
		expect(canvas.getByRole("tab", { name: "Terminal 2" })).toHaveAttribute(
			"aria-selected",
			"true",
		);

		await user.click(
			canvas.getByRole("button", { name: "Close Terminal 2 tab" }),
		);

		expect(
			canvas.queryByRole("tab", { name: "Terminal 2" }),
		).not.toBeInTheDocument();
		expect(canvas.getByRole("tab", { name: "Terminal 4" })).toHaveAttribute(
			"aria-selected",
			"true",
		);
	},
};

export const AddTabControlDisabled: Story = {
	args: {
		tabs: [gitTab],
		addTabControl: (
			<Button
				variant="outline"
				size="icon"
				onClick={fn()}
				disabled
				aria-label="New terminal tab"
				title="New terminal tab"
				className="size-6 bg-surface-primary p-0 text-content-secondary hover:text-content-primary"
			>
				<PlusIcon className="size-3.5" />
			</Button>
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: "New terminal tab" }),
		).toBeDisabled();
	},
};
