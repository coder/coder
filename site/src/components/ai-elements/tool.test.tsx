import { screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { createMemoryRouter } from "react-router";
import { renderWithRouter } from "testHelpers/renderHelpers";
import { Tool } from "./tool";

type ToolProps = ComponentProps<typeof Tool>;

const renderTool = (props: ToolProps) => {
	const router = createMemoryRouter(
		[
			{
				path: "/",
				element: <Tool {...props} />,
			},
			{
				path: "/agents/:agentId",
				element: <div>Agent page</div>,
			},
		],
		{ initialEntries: ["/"] },
	);

	return renderWithRouter(router);
};

describe(Tool.name, () => {
	it.each(["subagent", "subagent_await", "subagent_message"] as const)(
		"renders a Sub-agent link card for %s",
		(toolName) => {
			renderTool({
				name: toolName,
				args: { title: "Sub-agent" },
				result: { chat_id: "child-chat-id", status: "pending" },
			});

			expect(screen.getByRole("link", { name: "View agent" })).toHaveAttribute(
				"href",
				"/agents/child-chat-id",
			);
		},
	);

	it("maps pending delegated status to running rendering", () => {
		const { container } = renderTool({
			name: "subagent",
			result: { chat_id: "child-chat-id", status: "pending" },
			status: "completed",
		});

		expect(screen.getByRole("link", { name: "View agent" })).toHaveAttribute(
			"href",
			"/agents/child-chat-id",
		);
		expect(
			screen.getByRole("button", { name: /Spawning Sub-agent/ }),
		).toBeInTheDocument();
		expect(container.querySelector(".animate-spin")).toBeInTheDocument();
	});

	it("uses stream override status for delegated status rendering", () => {
		const { container } = renderTool({
			name: "subagent",
			result: { chat_id: "child-chat-id", status: "pending" },
			status: "completed",
			subagentStatusOverrides: new Map([
				["child-chat-id", "completed"],
			]),
		});

		expect(
			screen.getByRole("button", { name: /Spawned Sub-agent/ }),
		).toBeInTheDocument();
		expect(container.querySelector(".animate-spin")).toBeNull();
	});

	it("does not show a subagent error icon when completed despite parser noise", () => {
		const { container } = renderTool({
			name: "subagent",
			result: {
				chat_id: "child-chat-id",
				status: "completed",
				error: "provider metadata noise",
			},
			status: "error",
			isError: true,
		});

		expect(container.querySelector(".animate-spin")).toBeNull();
		expect(container.querySelector(".lucide-circle-alert")).toBeNull();
		expect(
			screen.getByRole("button", { name: /Spawned Sub-agent/ }),
		).toBeInTheDocument();
	});

	it("prefers returned subagent title for await tools", () => {
		renderTool({
			name: "subagent_await",
			args: { title: "Fallback title" },
			result: {
				chat_id: "child-chat-id",
				title: "Delegated child title",
				status: "completed",
			},
		});

		expect(screen.getByText("Delegated child title")).toBeInTheDocument();
		expect(screen.getByRole("link", { name: "View agent" })).toHaveAttribute(
			"href",
			"/agents/child-chat-id",
		);
		expect(screen.queryByText("Fallback title")).toBeNull();
	});

	it.each(["subagent", "subagent_await", "subagent_message"] as const)(
		"renders request metadata for %s when present",
		(toolName) => {
			renderTool({
				name: toolName,
				result: {
					chat_id: "child-chat-id",
					status: "completed",
					request_id: "request-123",
					duration_ms: 1530,
				},
			});

			expect(screen.getByText("Worked for 2s")).toBeInTheDocument();
		},
	);

	it("renders subagent_report output", () => {
		renderTool({
			name: "subagent_report",
			args: { report: "Done." },
			result: { title: "Sub-agent report" },
		});

		expect(screen.getByText("Sub-agent report")).toBeInTheDocument();
		expect(screen.getByText("Done.")).toBeInTheDocument();
	});

	it("renders subagent_terminate label", () => {
		renderTool({
			name: "subagent_terminate",
		});

		expect(screen.getByText(/Terminated/)).toBeInTheDocument();
		expect(screen.getByText("Sub-agent")).toBeInTheDocument();
	});

	it("does not use task-specific delegated rendering", () => {
		renderTool({
			name: "task",
		});

		expect(screen.getByText("task")).toBeInTheDocument();
		expect(screen.queryByRole("link", { name: "View agent" })).toBeNull();
	});
});
