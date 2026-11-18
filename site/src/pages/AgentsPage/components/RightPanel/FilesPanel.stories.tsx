import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type {
	WorkspaceAgentFile,
	WorkspaceAgentListDirectoryResponse,
} from "#/api/api";
import { workspaceAgentDirectoryKey } from "#/api/queries/workspaceFiles";
import { WorkspaceFilesContext } from "../../context/WorkspaceFilesContext";
import { FilesPanel } from "./FilesPanel";

const agentId = "story-agent";

const entry = (
	absolutePath: string,
	isDir: boolean,
	ignored = false,
): WorkspaceAgentFile => ({
	name: absolutePath.slice(absolutePath.lastIndexOf("/") + 1),
	absolute_path_string: absolutePath,
	is_dir: isDir,
	is_symlink: false,
	size: 0,
	mod_time: "2026-01-01T00:00:00Z",
	ignored,
});

const listing = (
	absolutePath: string,
	contents: WorkspaceAgentFile[],
): WorkspaceAgentListDirectoryResponse => ({
	absolute_path: absolutePath.split("/").filter(Boolean),
	absolute_path_string: absolutePath,
	contents,
});

const meta: Meta<typeof FilesPanel> = {
	title: "pages/AgentsPage/components/RightPanel/FilesPanel",
	component: FilesPanel,
	args: { chatId: "story-files-panel" },
	decorators: [
		(Story) => (
			<WorkspaceFilesContext
				value={{
					agentId,
					defaultDirectory: "/home/coder/repo",
					repoRoots: [],
					repoRootsSettled: true,
					repositories: new Map(),
					openFile: () => {},
					revealInFiles: () => {},
					diffStatuses: [],
				}}
			>
				<div className="flex h-[420px] w-80 flex-col border border-solid border-border">
					<Story />
				</div>
			</WorkspaceFilesContext>
		),
	],
};

export default meta;
type Story = StoryObj<typeof FilesPanel>;

export const IgnoredEntries: Story = {
	parameters: {
		queries: [
			{
				key: workspaceAgentDirectoryKey(agentId, ""),
				data: listing("/home/coder/repo", [
					entry("/home/coder/repo/node_modules", true, true),
					entry("/home/coder/repo/src", true),
					entry("/home/coder/repo/.env", false, true),
					entry("/home/coder/repo/README.md", false),
					entry("/home/coder/repo/package.json", false),
					entry("/home/coder/repo/index.ts", false),
					entry("/home/coder/repo/Makefile", false),
					entry("/home/coder/repo/debug.log", false, true),
				]),
			},
			{
				key: workspaceAgentDirectoryKey(agentId, "/home/coder/repo/src"),
				data: listing("/home/coder/repo/src", [
					entry("/home/coder/repo/src/dist", true, true),
					entry("/home/coder/repo/src/main.go", false),
					entry("/home/coder/repo/src/main.go.orig", false, true),
				]),
			},
		],
	},
	// Expands src so the screenshot covers ignored rows at both levels.
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(await canvas.findByRole("treeitem", { name: "src" }));
		await canvas.findByRole("treeitem", { name: "main.go" });
	},
};

export const RootPicker: Story = {
	args: { chatId: "story-files-panel-root-picker" },
	decorators: [
		(Story) => (
			<WorkspaceFilesContext
				value={{
					agentId,
					defaultDirectory: "/home/coder",
					repoRoots: ["/home/coder/coder", "/home/coder/demo/site"],
					repoRootsSettled: true,
					repositories: new Map(),
					openFile: () => {},
					revealInFiles: () => {},
					diffStatuses: [],
				}}
			>
				<Story />
			</WorkspaceFilesContext>
		),
	],
	parameters: {
		queries: [
			{
				key: workspaceAgentDirectoryKey(agentId, "/home/coder/coder"),
				data: listing("/home/coder/coder", [
					entry("/home/coder/coder/coderd", true),
					entry("/home/coder/coder/site", true),
					entry("/home/coder/coder/go.mod", false),
				]),
			},
			{
				key: workspaceAgentDirectoryKey(agentId, "~"),
				data: listing("/home/coder", [entry("/home/coder/coder", true)]),
			},
		],
	},
	// Opens the picker so the screenshot shows its groups and paths.
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", { name: /^Root directory/ }),
		);
	},
};
