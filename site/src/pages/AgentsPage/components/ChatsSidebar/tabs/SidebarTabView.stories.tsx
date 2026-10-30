import type { Meta, StoryObj } from "@storybook/react-vite";
import { PlusIcon } from "lucide-react";
import { useState } from "react";
import { fn, userEvent, within } from "storybook/test";
import { Button } from "#/components/Button/Button";
import { SidebarTab, SidebarTabView } from "./SidebarTabView";

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

const gitTab = (
	<SidebarTab id="git" label="Git" badge={makeBadge(42, 7)}>
		{makePanelContent("Git")}
	</SidebarTab>
);

const addTabButton = (disabled?: boolean) => (
	<Button
		variant="outline"
		size="icon"
		onClick={fn()}
		disabled={disabled}
		aria-label="New terminal tab"
		title="New terminal tab"
		className="size-6 bg-surface-primary p-0 text-content-secondary hover:text-content-primary"
	>
		<PlusIcon className="size-3.5" />
	</Button>
);

const meta: Meta<typeof SidebarTabView> = {
	title: "pages/AgentsPage/SidebarTabView",
	component: SidebarTabView,
	args: {
		children: gitTab,
		activeTabId: "git",
		onActiveTabChange: fn(),
		isPanelOpen: true,
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
		children: (
			<SidebarTab id="git" label="Git">
				{makePanelContent("Git")}
			</SidebarTab>
		),
	},
};

export const MultipleTabs: Story = {
	args: {
		children: [
			<SidebarTab key="git" id="git" label="Git" badge={makeBadge(42, 7)}>
				{makePanelContent("Git")}
			</SidebarTab>,
			<SidebarTab key="preview" id="preview" label="Preview">
				{makePanelContent("Preview")}
			</SidebarTab>,
		],
	},
};

export const EmptyState: Story = {
	args: {
		children: [],
	},
};

export const ExpandedWithTitle: Story = {
	args: {
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
		const [terminalIds, setTerminalIds] = useState(() =>
			Array.from({ length: 8 }, (_, index) => `terminal-${index + 2}`),
		);

		const handleCloseTab = (
			tabId: string,
			nextActiveTabId: string | undefined,
		) => {
			setTerminalIds(terminalIds.filter((id) => id !== tabId));

			if (nextActiveTabId) {
				setActiveTabId(nextActiveTabId);
			}
		};

		return (
			<SidebarTabView
				activeTabId={activeTabId}
				onActiveTabChange={setActiveTabId}
				isPanelOpen
				isExpanded={false}
				onToggleExpanded={() => {}}
				addTabControl={() => addTabButton()}
			>
				{gitTab}
				<SidebarTab id="terminal" label="Terminal">
					{makePanelContent("Terminal")}
				</SidebarTab>
				<SidebarTab id="summary" label="Summary">
					{makePanelContent("Summary")}
				</SidebarTab>
				{terminalIds.map((id) => {
					const label = `Terminal ${id.slice("terminal-".length)}`;

					return (
						<SidebarTab
							key={id}
							id={id}
							label={label}
							onClose={(next) => handleCloseTab(id, next)}
						>
							{makePanelContent(label)}
						</SidebarTab>
					);
				})}
			</SidebarTabView>
		);
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);

		// Close Terminal 3 then the active Terminal 2. Closing the active tab
		// selects its neighbor, Terminal 4.
		await user.click(
			canvas.getByRole("button", { name: "Close Terminal 3 tab" }),
		);

		await user.click(
			canvas.getByRole("button", { name: "Close Terminal 2 tab" }),
		);
	},
};

export const AddTabControlDisabled: Story = {
	args: {
		addTabControl: () => addTabButton(true),
	},
};
