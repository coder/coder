import type { Meta, StoryObj } from "@storybook/react-vite";
import * as monaco from "monaco-editor";
import { useEffect, useState } from "react";
import { userEvent, waitFor, within } from "storybook/test";
import type { WorkspaceAgentFileContents } from "#/api/api";
import { workspaceAgentFileKey } from "#/api/queries/workspaceFiles";
import type {
	ChatDiffStatus,
	WorkspaceAgentRepoChanges,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { setupMatchMedia } from "#/testHelpers/matchMedia";
import { noHoverMediaQuery } from "#/utils/mobile";
import { MessageScroller } from "#/vendor/message-scroller";
import { WorkspaceFilesContext } from "../../context/WorkspaceFilesContext";
import {
	type PersistedMainTabs,
	savePersistedMainTabs,
} from "../../utils/mainTabStorage";
import { createChatStore } from "../ChatConversation/chatStore";
import { ChatMessageScroller } from "../ChatMessageScroller";
import { MainTabs } from "./MainTabs";
import { useMainTabs } from "./useMainTabs";

const agentId = "story-agent";
const chatId = "story-main-tabs";

const textFile = (content: string): WorkspaceAgentFileContents => ({
	data: new TextEncoder().encode(content).buffer,
	contentType: "text/plain; charset=utf-8",
	etag: '"v1"',
});

const fileQueries = [
	{
		key: workspaceAgentFileKey(agentId, "/repo/src/index.ts"),
		data: textFile('export const greet = () => "hello";\n'),
	},
	{
		key: workspaceAgentFileKey(agentId, "/repo/test/index.ts"),
		data: textFile('import { greet } from "../src";\n'),
	},
	{
		key: workspaceAgentFileKey(agentId, "/repo/README.md"),
		data: textFile(
			"# Example\n\nA short README with a list:\n\n- one\n- two\n\n```sh\nmake build\n```\n",
		),
	},
];

const origin = "https://github.com/coder/example.git";
const branch = "feat/files";

const repositories = new Map<string, WorkspaceAgentRepoChanges>([
	[
		"/repo",
		{
			repo_root: "/repo",
			branch,
			remote_origin: origin,
			head_commit: "6f1c2d3e4b5a69788796a5b4c3d2e1f0a9b8c7d6",
		},
	],
]);

const diffStatuses: ChatDiffStatus[] = [
	{
		chat_id: chatId,
		remote_origin: origin,
		git_branch: branch,
		url: "https://github.com/coder/example/pull/42",
		pr_number: 42,
		pull_request_title: "Add a file browser",
		pull_request_draft: false,
		changes_requested: false,
		additions: 120,
		deletions: 8,
		changed_files: 6,
	},
];

const StoryMainTabs: React.FC<{ children?: React.ReactNode }> = ({
	children = <ChatPlaceholder />,
}) => {
	const mainTabs = useMainTabs(chatId);
	const [store] = useState(createChatStore);
	return (
		<WorkspaceFilesContext
			value={{
				agentId,
				defaultDirectory: "/repo",
				repoRoots: ["/repo"],
				repositories,
				openFile: mainTabs.openFile,
				revealInFiles: () => {},
				diffStatuses,
			}}
		>
			<div className="flex h-[480px] flex-col">
				<MainTabs
					mainTabs={mainTabs}
					store={store}
					readOnly={false}
					filesEnabled
					focusChatInput={() => {}}
				>
					{children}
				</MainTabs>
			</div>
		</WorkspaceFilesContext>
	);
};

const ChatPlaceholder: React.FC = () => (
	<div className="relative flex flex-1 items-center justify-center text-sm text-content-secondary">
		Chat transcript
		{/* Stands in for a control with Tailwind's `visible`, such as one that
		    touch screens show without hover. It must not show over an active
		    file tab. */}
		<div className="visible absolute right-4 top-4 z-10 rounded-md border border-solid border-border bg-surface-secondary px-3 py-2 text-xs">
			Floating chat widget
		</div>
	</div>
);

const replyLineCount = 60;

/**
 * A transcript in the real scroller, with controls outside the tabs that
 * send a prompt and stream its reply while a file tab hides the chat.
 */
const StreamingChat: React.FC = () => {
	const [replyLines, setReplyLines] = useState<number>();
	const [isStreaming, setIsStreaming] = useState(false);
	const isComplete = replyLines === replyLineCount;

	useEffect(() => {
		if (!isStreaming || isComplete) {
			return;
		}
		const timer = window.setTimeout(
			() => setReplyLines((lines) => (lines ?? 0) + 1),
			20,
		);
		return () => window.clearTimeout(timer);
	}, [isStreaming, isComplete, replyLines]);

	return (
		<MessageScroller.Provider autoScroll defaultScrollPosition="end">
			<div className="flex shrink-0 items-center gap-2 p-2">
				<Button
					size="sm"
					variant="outline"
					disabled={replyLines !== undefined}
					onClick={() => setReplyLines(0)}
				>
					Send message
				</Button>
				<Button
					size="sm"
					variant="outline"
					disabled={replyLines === undefined || isStreaming}
					onClick={() => setIsStreaming(true)}
				>
					Stream reply
				</Button>
				<span className="text-xs text-content-secondary">
					{isComplete && "Reply complete"}
				</span>
			</div>
			<StoryMainTabs>
				<ChatMessageScroller
					hasMoreMessages={false}
					isFetchingMoreMessages={false}
					isHydratingMessages={false}
					hasFetchMoreError={false}
					hasTranscriptRows
					onFetchMoreMessages={async () => {}}
				>
					{Array.from({ length: 6 }, (_, turn) => (
						<MessageScroller.Item key={turn} messageId={`history-${turn}`}>
							<p className="m-0 text-sm text-content-secondary">
								Earlier turn {turn + 1}
							</p>
						</MessageScroller.Item>
					))}
					{replyLines !== undefined && (
						<>
							<MessageScroller.Item messageId="prompt" scrollAnchor>
								<p className="m-0 rounded-md bg-surface-secondary px-3 py-2 text-sm">
									Summarize the build output
								</p>
							</MessageScroller.Item>
							<MessageScroller.Item messageId="reply">
								{Array.from({ length: replyLines }, (_, line) => (
									<p key={line} className="m-0 text-sm">
										Reply line {line + 1}
									</p>
								))}
							</MessageScroller.Item>
						</>
					)}
				</ChatMessageScroller>
			</StoryMainTabs>
		</MessageScroller.Provider>
	);
};

const persistTabs = (tabs: PersistedMainTabs) => () => {
	savePersistedMainTabs(chatId, tabs);
	return () => savePersistedMainTabs(chatId, { paths: [], activePath: null });
};

const meta: Meta<typeof StoryMainTabs> = {
	title: "pages/AgentsPage/MainTabs",
	component: StoryMainTabs,
	parameters: { queries: fileQueries },
};

export default meta;
type Story = StoryObj<typeof StoryMainTabs>;

export const ChatActive: Story = {
	beforeEach: persistTabs({
		paths: ["/repo/src/index.ts", "/repo/test/index.ts", "/repo/README.md"],
		activePath: null,
	}),
};

export const MarkdownPreview: Story = {
	beforeEach: persistTabs({
		paths: ["/repo/src/index.ts", "/repo/README.md"],
		activePath: "/repo/README.md",
	}),
};

/**
 * The reply outgrows the space below its prompt while a file tab hides the
 * chat. The chat must follow it to the end, not stay on the prompt.
 */
export const ChatFollowsReplyWhileHidden: Story = {
	render: () => <StreamingChat />,
	beforeEach: persistTabs({ paths: ["/repo/README.md"], activePath: null }),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Send message" }));
		// The scroller anchors the prompt on an animation frame.
		await new Promise((resolve) =>
			requestAnimationFrame(() => requestAnimationFrame(resolve)),
		);
		await userEvent.click(canvas.getByRole("tab", { name: "README.md" }));
		await userEvent.click(canvas.getByRole("button", { name: "Stream reply" }));
		await canvas.findByText("Reply complete", undefined, { timeout: 5000 });
		await userEvent.click(canvas.getByRole("tab", { name: "Chat" }));
	},
};

/** Edits the file's editor through Monaco, which synthetic typing cannot reach. */
const editFile = async (path: string) => {
	const editor = await waitFor(
		() => {
			const found = monaco.editor
				.getEditors()
				.find((candidate) => candidate.getModel()?.uri.path === path);
			// The file tab hears about edits from when the editor turns opaque.
			if (!found || found.getContainerDomNode().closest(".opacity-0")) {
				throw new Error(`No editor is ready for ${path} yet.`);
			}
			return found;
		},
		{ timeout: 10000 },
	);
	editor.executeEdits("story", [
		{ range: new monaco.Range(1, 1, 1, 1), text: "// edited\n" },
	]);
	// The edit moves the cursor, which starts Monaco's 50 ms word highlight
	// delay. Unmounting the story before it ends rejects an unhandled promise.
	await new Promise((resolve) => setTimeout(resolve, 100));
};

const unsavedTabs: PersistedMainTabs = {
	paths: ["/repo/README.md", "/repo/src/index.ts"],
	activePath: "/repo/src/index.ts",
};

export const UnsavedFile: Story = {
	beforeEach: persistTabs(unsavedTabs),
	play: () => editFile("/repo/src/index.ts"),
};

export const UnsavedFileFocused: Story = {
	beforeEach: persistTabs(unsavedTabs),
	play: async ({ canvasElement }) => {
		await editFile("/repo/src/index.ts");
		const tab = await within(canvasElement).findByRole("tab", {
			name: "index.ts, unsaved",
		});
		tab.focus();
		// A key press makes the focus ring show; End keeps the last tab active.
		await userEvent.keyboard("{End}");
	},
};

/** Touch screens cannot hover, so the close button always shows its X. */
export const UnsavedFileWithoutHover: Story = {
	beforeEach: () => {
		const media = setupMatchMedia({ [noHoverMediaQuery]: true });
		const restoreTabs = persistTabs(unsavedTabs)();
		return () => {
			media.restore();
			restoreTabs();
		};
	},
	play: () => editFile("/repo/src/index.ts"),
};

export const FileActionsMenu: Story = {
	beforeEach: persistTabs({
		paths: ["/repo/src/index.ts", "/repo/README.md"],
		activePath: "/repo/src/index.ts",
	}),
	play: async ({ canvasElement }) => {
		await userEvent.click(
			await within(canvasElement).findByRole("button", {
				name: "File actions",
			}),
		);
	},
};

/** The menu of more tabs than fit the screen scrolls inside it. */
export const AllTabsMenu: Story = {
	beforeEach: persistTabs({
		paths: Array.from({ length: 40 }, (_, i) => `/repo/src/module-${i + 1}.ts`),
		activePath: null,
	}),
	play: async ({ canvasElement }) => {
		await userEvent.click(
			await within(canvasElement).findByRole("button", { name: "All tabs" }),
		);
	},
};
