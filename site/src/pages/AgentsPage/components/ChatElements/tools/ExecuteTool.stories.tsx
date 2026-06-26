import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { ExecuteTool } from "./ExecuteTool";

const longCommand =
	"find /home/coder/project/src -type f -name '*.ts' -not -path '*/node_modules/*' -not -path '*/.git/*' | xargs grep -l 'deprecated' | sort | head -50";

const stoppedWorkspaceError =
	"workspace has no running agent: the workspace is likely stopped. Use the start_workspace tool to start it";

const meta: Meta<typeof ExecuteTool> = {
	title: "components/ai-elements/tool/ExecuteTool",
	component: ExecuteTool,
	args: {
		status: "completed",
		isError: false,
		transcriptBlocks: [],
	},
};
export default meta;
type Story = StoryObj<typeof ExecuteTool>;

/** A short command that fits on a single line without truncation. */
export const ShortCommand: Story = {
	args: {
		command: "git status",
		transcriptBlocks: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const summaryButton = await canvas.findByRole("button", {
			name: "Expand command",
		});
		await userEvent.click(summaryButton);
	},
};

export const RunningWithoutCommand: Story = {
	args: {
		command: "",
		status: "running",
		transcriptBlocks: [],
	},
};

export const LongCommand: Story = {
	decorators: [
		(Story) => (
			<div className="w-72">
				<Story />
			</div>
		),
	],
	args: {
		command: longCommand,
		transcriptBlocks: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const summaryButton = await canvas.findByRole("button", {
			name: "Expand command",
		});
		await userEvent.click(summaryButton);
	},
};

/** A long truncated command with multi-line output below it. */
export const LongCommandWithOutput: Story = {
	args: {
		command: longCommand,
		transcriptBlocks: [
			{
				kind: "output",
				text: [
					"src/api/legacyClient.ts",
					"src/components/OldTable/OldTable.tsx",
					"src/hooks/useObsoleteAuth.ts",
					"src/pages/SettingsPage/DeprecatedPanel.tsx",
					"src/utils/formatDate.ts",
					"src/utils/legacyHelpers.ts",
				].join("\n"),
			},
		],
	},
};

/** A normal command with multi-line output that overflows the collapsed preview. */
export const WithOutput: Story = {
	args: {
		command: "docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'",
		transcriptBlocks: [
			{
				kind: "output",
				text: [
					"NAMES                STATUS              PORTS",
					"coder-gateway        Up 3 hours          0.0.0.0:3000->3000/tcp",
					"coder-database       Up 3 hours          0.0.0.0:5432->5432/tcp",
					"coder-provisioner    Up 3 hours",
					"redis-cache          Up 3 hours          0.0.0.0:6379->6379/tcp",
					"nginx-proxy          Up 2 hours          0.0.0.0:80->80/tcp, 0.0.0.0:443->443/tcp",
					"prometheus           Up 2 hours          0.0.0.0:9090->9090/tcp",
					"grafana              Up 2 hours          0.0.0.0:3001->3001/tcp",
					"jaeger               Up 1 hour           0.0.0.0:16686->16686/tcp",
					"otel-collector       Up 1 hour           0.0.0.0:4317->4317/tcp",
					"loki                 Up 1 hour           0.0.0.0:3100->3100/tcp",
				].join("\n"),
			},
		],
	},
};

/** A command currently running shows a spinner in the header. */
export const Running: Story = {
	args: {
		command: "go test -race -count=1 ./coderd/...",
		status: "running",
		transcriptBlocks: [
			{
				kind: "output",
				text: "=== RUN   TestWorkspaceAgent\n--- PASS: TestWorkspaceAgent (0.42s)",
			},
		],
	},
};

/** A command that errored renders the output in red. */
export const ErrorOutput: Story = {
	args: {
		command: "make build",
		status: "completed",
		isError: true,
		transcriptBlocks: [
			{
				kind: "output",
				text: [
					"coderd/workspaces.go:142:6: cannot use ws (variable of type *database.Workspace) as database.Store value in argument to api.Authorize",
					"coderd/workspaces.go:155:19: ws.OwnerID undefined (type *database.Workspace has no field or method OwnerID)",
					"make: *** [build] Error 1",
				].join("\n"),
			},
		],
	},
};

/** A connection error renders its details inside the expanded transcript. */
export const ConnectionError: Story = {
	args: {
		command: "ls -la",
		status: "error",
		isError: true,
		shellToolDisplayMode: "auto",
		transcriptBlocks: [{ kind: "error", text: stoppedWorkspaceError }],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const errorMessage = await canvas.findByText(
			/workspace has no running agent/i,
		);
		await expect(errorMessage).toBeVisible();
	},
};

/** A timed-out command can return partial output plus an execute error. */
export const OutputWithError: Story = {
	args: {
		command: "go test ./...",
		status: "completed",
		isBackgrounded: true,
		transcriptBlocks: [
			{
				kind: "output",
				text: [
					"=== RUN   TestWorkspaceAgent",
					"--- PASS: TestWorkspaceAgent (0.42s)",
					"=== RUN   TestWorkspaceBuild",
				].join("\n"),
			},
			{ kind: "error", text: "command timed out after 10s" },
		],
	},
};

/** parsedCommands replaces the raw command in the summary line. */
export const ParsedCommands: Story = {
	args: {
		command: `cd /repo && git pull && git add . && git commit -m "fix bug"`,
		status: "completed",
		durationMs: 3200,
		parsedCommands: [
			["cd", "/repo"],
			["git", "pull"],
			["git", "add"],
			["git", "commit"],
		],
	},
};

/** parsedCommands paired with modelIntent. */
export const ParsedCommandsWithIntent: Story = {
	args: {
		command: "cd /repo && go test -race ./coderd/...",
		status: "running",
		modelIntent: "Running the unit tests",
		parsedCommands: [
			["cd", "/repo"],
			["go", "test"],
		],
	},
};

export const LongUnbrokenLineOutput: Story = {
	decorators: [
		(Story) => (
			<div className="w-72">
				<Story />
			</div>
		),
	],
	args: {
		command: "cat access-token.txt",
		transcriptBlocks: [
			{
				kind: "output",
				text: `token:${"A".repeat(400)}:end`,
			},
		],
	},
	play: async ({ canvasElement }) => {
		const viewport = canvasElement.querySelector<HTMLElement>(
			"[data-radix-scroll-area-viewport]",
		);
		await expect(viewport).not.toBeNull();
		if (viewport) {
			await expect(viewport.scrollWidth).toBeLessThanOrEqual(
				viewport.clientWidth + 2,
			);
		}
	},
};
