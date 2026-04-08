import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { ExecuteTool } from "./ExecuteTool";

const meta: Meta<typeof ExecuteTool> = {
	title: "components/ai-elements/tool/ExecuteTool",
	component: ExecuteTool,
	decorators: [
		(Story) => (
			<div className="max-w-3xl rounded-lg border border-solid border-border-default bg-surface-primary p-4">
				<Story />
			</div>
		),
	],
	args: {
		status: "completed",
		isError: false,
		output: "",
	},
};
export default meta;
type Story = StoryObj<typeof ExecuteTool>;

/** A short command that fits on a single line without truncation. */
export const ShortCommand: Story = {
	args: {
		command: "git status",
		output: "",
	},
};

/** A long command expanded to show the full text, with the chevron visible on hover. */
export const LongCommand: Story = {
	args: {
		command:
			"find /home/coder/project/src -type f -name '*.ts' -not -path '*/node_modules/*' -not -path '*/.git/*' | xargs grep -l 'deprecated' | sort | head -50",
		output: "",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const chevron = canvas.getByRole("button", {
			name: /expand command/i,
		});
		await userEvent.click(chevron);
		// Hover the component so the chevron stays visible.
		await userEvent.hover(canvasElement.firstElementChild!);
	},
};

/** A long truncated command with multi-line output below it. */
export const LongCommandWithOutput: Story = {
	args: {
		command:
			"find /home/coder/project/src -type f -name '*.ts' -not -path '*/node_modules/*' -not -path '*/.git/*' | xargs grep -l 'deprecated' | sort | head -50",
		output: [
			"src/api/legacyClient.ts",
			"src/components/OldTable/OldTable.tsx",
			"src/hooks/useObsoleteAuth.ts",
			"src/pages/SettingsPage/DeprecatedPanel.tsx",
			"src/utils/formatDate.ts",
			"src/utils/legacyHelpers.ts",
		].join("\n"),
	},
};

/** A normal command with multi-line output that overflows the collapsed preview. */
export const WithOutput: Story = {
	args: {
		command: "docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'",
		output: [
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
};

/** A command currently running shows a spinner in the header. */
export const Running: Story = {
	args: {
		command: "go test -race -count=1 ./coderd/...",
		status: "running",
		output:
			"=== RUN   TestWorkspaceAgent\n--- PASS: TestWorkspaceAgent (0.42s)",
	},
};

/** A command that errored renders the output in red. */
export const ErrorOutput: Story = {
	args: {
		command: "make build",
		status: "completed",
		isError: true,
		output: [
			"coderd/workspaces.go:142:6: cannot use ws (variable of type *database.Workspace) as database.Store value in argument to api.Authorize",
			"coderd/workspaces.go:155:19: ws.OwnerID undefined (type *database.Workspace has no field or method OwnerID)",
			"make: *** [build] Error 1",
		].join("\n"),
	},
};
