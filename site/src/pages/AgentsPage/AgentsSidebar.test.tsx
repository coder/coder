import type { Chat } from "api/typesGenerated";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { renderWithRouter } from "testHelpers/renderHelpers";
import { AgentsSidebar } from "./AgentsSidebar";

type SidebarChat = Chat & {
	readonly parent_chat_id?: string;
	readonly root_chat_id?: string;
	readonly task_status?: "queued" | "running" | "awaiting_report" | "reported";
};

const defaultModelOptions = [
	{
		id: "openai:gpt-4o",
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
] as const;

const buildChat = (overrides: Partial<SidebarChat>): SidebarChat => {
	return {
		id: "chat-default",
		owner_id: "owner-1",
		title: "Agent",
		status: "completed",
		model_config: {
			provider: "openai",
			model: "gpt-4o",
		},
		created_at: "2026-02-18T00:00:00.000Z",
		updated_at: "2026-02-18T00:00:00.000Z",
		...overrides,
	};
};

const renderSidebar = ({
	chats,
	initialPath = "/agents",
}: {
	readonly chats: readonly SidebarChat[];
	readonly initialPath?: string;
}) => {
	const router = createMemoryRouter(
		[
			{
				path: "/agents",
				element: (
					<AgentsSidebar
						chats={chats}
						chatErrorReasons={{}}
						modelOptions={defaultModelOptions}
						onArchiveAgent={vi.fn()}
						onNewAgent={vi.fn()}
						isCreating={false}
					/>
				),
			},
			{
				path: "/agents/:agentId",
				element: (
					<AgentsSidebar
						chats={chats}
						chatErrorReasons={{}}
						modelOptions={defaultModelOptions}
						onArchiveAgent={vi.fn()}
						onNewAgent={vi.fn()}
						isCreating={false}
					/>
				),
			},
			{ path: "/workspaces", element: <div>Workspaces</div> },
		],
		{ initialEntries: [initialPath] },
	);

	return renderWithRouter(router);
};

describe(AgentsSidebar.name, () => {
	it("renders hierarchy and keeps parent context in search results", async () => {
		const chats = [
			buildChat({ id: "parent-1", title: "Parent planner" }),
			buildChat({
				id: "child-1",
				title: "Child executor",
				parent_chat_id: "parent-1",
				root_chat_id: "parent-1",
				task_status: "queued",
			}),
		];
		renderSidebar({ chats });

		const user = userEvent.setup();
		await user.type(screen.getByPlaceholderText("Search agents..."), "child");

		expect(screen.getByText("Parent planner")).toBeInTheDocument();
		expect(screen.getByText("Child executor")).toBeInTheDocument();
	});

	it("shows executing indicator for running delegated chats", () => {
		const chats = [
			buildChat({ id: "root-1", title: "Root agent" }),
			buildChat({
				id: "child-running",
				title: "Running child",
				status: "running",
				parent_chat_id: "root-1",
				root_chat_id: "root-1",
				task_status: "running",
			}),
		];
		renderSidebar({ chats });

		expect(
			screen.getByTestId("agents-tree-executing-child-running"),
		).toBeInTheDocument();
	});

	it("supports expand and collapse for parent nodes", async () => {
		const chats = [
			buildChat({ id: "root-2", title: "Root for collapse" }),
			buildChat({
				id: "child-collapse",
				title: "Nested child",
				parent_chat_id: "root-2",
				root_chat_id: "root-2",
				task_status: "reported",
			}),
		];
		renderSidebar({ chats });

		const user = userEvent.setup();
		const toggle = screen.getByTestId("agents-tree-toggle-root-2");

		expect(toggle).toHaveAttribute("aria-expanded", "true");
		expect(screen.getByText("Nested child")).toBeInTheDocument();

		await user.click(toggle);
		expect(toggle).toHaveAttribute("aria-expanded", "false");
		expect(screen.queryByText("Nested child")).not.toBeInTheDocument();

		await user.click(toggle);
		expect(toggle).toHaveAttribute("aria-expanded", "true");
		expect(screen.getByText("Nested child")).toBeInTheDocument();
	});

	it("expands the active chat ancestry by default", () => {
		const chats = [
			buildChat({ id: "root-active", title: "Active root" }),
			buildChat({
				id: "child-active",
				title: "Active middle",
				parent_chat_id: "root-active",
				root_chat_id: "root-active",
				task_status: "awaiting_report",
			}),
			buildChat({
				id: "grandchild-active",
				title: "Active leaf",
				parent_chat_id: "child-active",
				root_chat_id: "root-active",
				task_status: "reported",
			}),
			buildChat({ id: "other-root", title: "Other root" }),
		];
		renderSidebar({ chats, initialPath: "/agents/grandchild-active" });

		expect(screen.getByText("Active root")).toBeInTheDocument();
		expect(screen.getByText("Active middle")).toBeInTheDocument();
		expect(screen.getByText("Active leaf")).toBeInTheDocument();
		expect(screen.getByTestId("agents-tree-toggle-root-active")).toHaveAttribute(
			"aria-expanded",
			"true",
		);
		expect(
			screen.getByTestId("agents-tree-toggle-child-active"),
		).toHaveAttribute("aria-expanded", "true");
	});
});
