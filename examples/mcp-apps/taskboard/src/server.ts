import { readFile } from "node:fs/promises";
import { createServer } from "node:http";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
	RESOURCE_MIME_TYPE,
	registerAppResource,
	registerAppTool,
} from "@modelcontextprotocol/ext-apps/server";
import { toNodeHandler } from "@modelcontextprotocol/node";
import {
	type CallToolResult,
	McpServer,
	createMcpHandler,
} from "@modelcontextprotocol/server";
import { z } from "zod";

interface Task {
	id: number;
	title: string;
	done: boolean;
}

const BOARD_URI = "ui://taskboard/board";
const BOARD_HTML_PATH = join(
	dirname(fileURLToPath(import.meta.url)),
	"board.html",
);
const PORT = Number(process.env.PORT ?? 3333);

// Module-level state so every MCP session sees the same board.
const tasks: Task[] = [];
let nextId = 1;

function summarize(): string {
	if (tasks.length === 0) {
		return "The task board is empty.";
	}
	const lines = tasks.map(
		(t) => `${t.done ? "[x]" : "[ ]"} #${t.id} ${t.title}`,
	);
	const noun = tasks.length === 1 ? "task" : "tasks";
	return `Task board (${tasks.length} ${noun}):\n${lines.join("\n")}`;
}

function result(prefix?: string): CallToolResult {
	const text = prefix ? `${prefix}\n\n${summarize()}` : summarize();
	return {
		content: [{ type: "text", text }],
		structuredContent: { tasks },
	};
}

function errorResult(message: string): CallToolResult {
	return {
		content: [{ type: "text", text: message }],
		structuredContent: { tasks },
		isError: true,
	};
}

// Tools carry _meta.ui so hosts render the board resource alongside results.
// visibility includes both "model" (agent may call) and "app" (board may call).
const uiMeta = {
	ui: { resourceUri: BOARD_URI, visibility: ["model", "app"] as const },
};

function buildServer(): McpServer {
	const server = new McpServer({ name: "taskboard", version: "0.1.0" });

	registerAppTool(
		server,
		"add_task",
		{
			title: "Add task",
			description: "Add a task to the shared task board.",
			inputSchema: z.object({ title: z.string().min(1) }),
			_meta: uiMeta,
		},
		async ({ title }) => {
			const task: Task = { id: nextId++, title, done: false };
			tasks.push(task);
			return result(`Added task #${task.id}: ${task.title}`);
		},
	);

	registerAppTool(
		server,
		"complete_task",
		{
			title: "Complete task",
			description: "Mark a task on the board as done by id.",
			inputSchema: z.object({ id: z.number().int() }),
			_meta: uiMeta,
		},
		async ({ id }) => {
			const task = tasks.find((t) => t.id === id);
			if (!task) {
				return errorResult(`No task with id ${id}.`);
			}
			task.done = true;
			return result(`Completed task #${task.id}: ${task.title}`);
		},
	);

	registerAppTool(
		server,
		"delete_task",
		{
			title: "Delete task",
			description: "Remove a task from the board by id.",
			inputSchema: z.object({ id: z.number().int() }),
			_meta: uiMeta,
		},
		async ({ id }) => {
			const index = tasks.findIndex((t) => t.id === id);
			if (index === -1) {
				return errorResult(`No task with id ${id}.`);
			}
			const [task] = tasks.splice(index, 1);
			return result(`Deleted task #${task.id}: ${task.title}`);
		},
	);

	registerAppTool(
		server,
		"list_tasks",
		{
			title: "List tasks",
			description: "List all tasks on the shared task board.",
			inputSchema: z.object({}),
			_meta: uiMeta,
		},
		async () => result(),
	);

	registerAppResource(
		server,
		"Task board",
		BOARD_URI,
		{
			description: "Interactive task board view.",
			mimeType: RESOURCE_MIME_TYPE,
			_meta: { ui: { prefersBorder: true } },
		},
		async () => ({
			contents: [
				{
					uri: BOARD_URI,
					mimeType: RESOURCE_MIME_TYPE,
					text: await readFile(BOARD_HTML_PATH, "utf8"),
					_meta: { ui: { prefersBorder: true } },
				},
			],
		}),
	);

	return server;
}

// The handler builds a fresh McpServer per request (stateless Streamable HTTP);
// the shared task list lives at module scope.
const mcp = toNodeHandler(createMcpHandler(() => buildServer()));

const httpServer = createServer((req, res) => {
	const path = new URL(req.url ?? "/", "http://localhost").pathname;
	if (path !== "/mcp") {
		res.writeHead(404, { "content-type": "text/plain" });
		res.end("not found");
		return;
	}
	void mcp(req, res);
});

httpServer.listen(PORT, () => {
	console.log(`taskboard MCP server listening on http://localhost:${PORT}/mcp`);
});
