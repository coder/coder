import type { Meta, StoryObj } from "@storybook/react-vite";
import { PlusIcon } from "lucide-react";
import { useState } from "react";
import { fn, userEvent, within } from "storybook/test";
import { Button } from "#/components/Button/Button";
import type { SidebarTab } from "./SidebarTabView";
import { SidebarTabView } from "./SidebarTabView";

const makePanelContent = (label: string) => (
	<div className="flex h-full items-center justify-center p-6 text-sm text-content-secondary">
		Content for {label}
	</div>
);

const makeTab = (id: string, label: string): SidebarTab => ({
	id,
	label,
	content: makePanelContent(label),
});

const gitTab = makeTab("git", "Git");
const summaryTab = makeTab("summary", "Summary");

const addTabControl = (
	<Button
		variant="outline"
		size="icon"
		onClick={fn()}
		aria-label="Add panel"
		title="Add panel"
		className="size-8 shrink-0 text-content-secondary hover:text-content-primary"
	>
		<PlusIcon className="size-4" />
	</Button>
);

const meta: Meta<typeof SidebarTabView> = {
	title: "pages/AgentsPage/SidebarTabView",
	component: SidebarTabView,
	args: {
		tabs: [summaryTab, gitTab],
		effectiveTabId: "git",
		onActiveTabChange: fn(),
		onReorder: fn(),
		isExpanded: false,
		onToggleExpanded: fn(),
		addTabControl,
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

export const Default: Story = {};

export const MultipleTabs: Story = {
	args: {
		tabs: [
			summaryTab,
			gitTab,
			makeTab("desktop", "Desktop"),
			{ ...makeTab("terminal", "Terminal"), onClose: fn() },
			{ ...makeTab("terminal-4", "Terminal"), badge: "4", onClose: fn() },
		],
	},
};

export const EmptyState: Story = {
	args: {
		tabs: [],
		addTabControl: undefined,
	},
};

export const ExpandedWithTitle: Story = {
	args: {
		tabs: [
			summaryTab,
			gitTab,
			{ ...makeTab("terminal", "Terminal"), onClose: fn() },
		],
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
		tabs: [
			summaryTab,
			gitTab,
			{ ...makeTab("terminal", "Terminal"), onClose: fn() },
			{ ...makeTab("terminal-2", "Terminal"), badge: "2", onClose: fn() },
		],
	},
	decorators: [
		(Story) => (
			<div style={{ height: 500, width: 360 }}>
				<Story />
			</div>
		),
	],
};

/** Hovering a closeable tab reveals its close button over the faded label. */
export const HoveredCloseableTab: Story = {
	args: {
		tabs: [
			summaryTab,
			gitTab,
			{
				...makeTab("preview", "Debug websocket disconnect in preview app"),
				onClose: fn(),
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.hover(
			canvas.getByRole("tab", {
				name: "Debug websocket disconnect in preview app",
			}),
		);
	},
};

export const AllTabsMenuOpen: Story = {
	args: {
		tabs: [
			summaryTab,
			gitTab,
			{ ...makeTab("terminal", "Terminal"), onClose: fn() },
			{ ...makeTab("terminal-2", "Terminal"), badge: "2", onClose: fn() },
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "All tabs" }));
		await within(document.body).findByRole("menuitemradio", { name: "Git" });
	},
};

/** Tabs can be dragged to a new position; the parent owns the order. */
export const ReorderableTabs: Story = {
	render: function ReorderableTabs() {
		const [tabs, setTabs] = useState<SidebarTab[]>([
			summaryTab,
			gitTab,
			makeTab("desktop", "Desktop"),
			{ ...makeTab("terminal", "Terminal"), onClose: fn() },
		]);
		const [activeTabId, setActiveTabId] = useState("git");
		return (
			<SidebarTabView
				tabs={tabs}
				effectiveTabId={activeTabId}
				onActiveTabChange={setActiveTabId}
				onReorder={(ids) =>
					setTabs((current) =>
						ids.flatMap((id) => current.filter((tab) => tab.id === id)),
					)
				}
				isExpanded={false}
				onToggleExpanded={() => {}}
				addTabControl={addTabControl}
			/>
		);
	},
};

export const CloseableTabs: Story = {
	render: function CloseableTabs() {
		const [activeTabId, setActiveTabId] = useState("terminal-2");
		const [tabs, setTabs] = useState<SidebarTab[]>([
			summaryTab,
			gitTab,
			makeTab("terminal", "Terminal"),
			...Array.from({ length: 8 }, (_, index) => ({
				...makeTab(`terminal-${index + 2}`, "Terminal"),
				badge: String(index + 2),
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
				addTabControl={addTabControl}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);

		// Close Terminal 3 then the active Terminal 2. The fixture's
		// close-then-select-neighbor logic activates Terminal 4.
		await user.click(
			canvas.getByRole("button", { name: "Close Terminal 3 tab" }),
		);

		await user.click(
			canvas.getByRole("button", { name: "Close Terminal 2 tab" }),
		);
	},
};
