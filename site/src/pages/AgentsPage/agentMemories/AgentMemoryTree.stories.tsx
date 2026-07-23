import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ComponentProps, useState } from "react";
import {
	type InfiniteData,
	QueryClient,
	QueryClientProvider,
} from "react-query";
import { expect, fn, userEvent, within } from "storybook/test";
import { agentMemoryChildren } from "#/api/queries/agentMemories";
import type {
	AgentMemoryChildrenResponse,
	AgentMemoryEntry,
} from "#/api/typesGenerated";
import { AgentMemoryTree } from "./AgentMemoryTree";

const memory = (path: string, id: string): AgentMemoryEntry => ({
	kind: "memory",
	path,
	id,
});

const directory = (path: string): AgentMemoryEntry => ({
	kind: "directory",
	path,
});

const entriesByDirectory: Record<string, readonly AgentMemoryEntry[]> = {
	"/": [directory("/daily"), directory("/projects"), memory("/memory.md", "1")],
	"/daily": [
		memory("/daily/2026-07-22.md", "2"),
		memory("/daily/2026-07-23.md", "3"),
	],
	"/projects": [directory("/projects/coder")],
	"/projects/coder": [memory("/projects/coder/design-notes.md", "4")],
};

const queryClient = new QueryClient({
	defaultOptions: {
		queries: {
			gcTime: Number.POSITIVE_INFINITY,
			refetchOnWindowFocus: false,
			retry: false,
			staleTime: Number.POSITIVE_INFINITY,
		},
	},
});

for (const [path, entries] of Object.entries(entriesByDirectory)) {
	const data: InfiniteData<AgentMemoryChildrenResponse> = {
		pages: [{ entries }],
		pageParams: [0],
	};
	queryClient.setQueryData(
		agentMemoryChildren(path, 0).queryKey.slice(0, -1),
		data,
	);
}

const MemoryTreeStory = (props: ComponentProps<typeof AgentMemoryTree>) => {
	const [expanded, setExpanded] = useState(props.expanded);
	return (
		<div className="w-72 rounded-lg border border-border-default bg-surface-primary p-2">
			<AgentMemoryTree
				{...props}
				expanded={expanded}
				onToggle={(path) => {
					setExpanded((current) => {
						const next = new Set(current);
						if (next.has(path)) next.delete(path);
						else next.add(path);
						return next;
					});
					props.onToggle(path);
				}}
			/>
		</div>
	);
};

const meta = {
	title: "pages/AgentsPage/AgentMemoryTree",
	component: AgentMemoryTree,
	decorators: [
		(Story) => (
			<QueryClientProvider client={queryClient}>
				<Story />
			</QueryClientProvider>
		),
	],
	args: {
		selectedPath: "/daily/2026-07-23.md",
		expanded: new Set(["/daily", "/projects", "/projects/coder"]),
		onToggle: fn(),
		onSelect: fn(),
	},
	render: (args) => <MemoryTreeStory {...args} />,
} satisfies Meta<typeof AgentMemoryTree>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Populated: Story = {};

export const CollapsesDirectory: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByRole("treeitem", { name: "daily" })).toBeVisible();
		expect(
			canvas.getByRole("treeitem", { name: "2026-07-23.md" }),
		).toHaveAttribute("aria-selected", "true");

		await userEvent.click(canvas.getByRole("treeitem", { name: "daily" }));
		expect(args.onToggle).toHaveBeenCalledWith("/daily");
		expect(
			canvas.queryByRole("treeitem", { name: "2026-07-23.md" }),
		).not.toBeInTheDocument();
	},
};
