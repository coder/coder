import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { WorkspaceAgentFileContents } from "#/api/api";
import { workspaceAgentFileKey } from "#/api/queries/workspaceFiles";
import { WorkspaceFilesContext } from "../../context/WorkspaceFilesContext";
import {
	type PersistedMainTabs,
	savePersistedMainTabs,
} from "../../utils/mainTabStorage";
import { createChatStore } from "../ChatConversation/chatStore";
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

const StoryMainTabs: React.FC = () => {
	const mainTabs = useMainTabs(chatId);
	const [store] = useState(createChatStore);
	return (
		<WorkspaceFilesContext
			value={{
				agentId,
				defaultDirectory: "/repo",
				repoRoots: [],
				repositories: new Map(),
				openFile: mainTabs.openFile,
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
					<div className="relative flex flex-1 items-center justify-center text-sm text-content-secondary">
						Chat transcript
						{/* Stands in for Monaco widgets, whose `visible` class matches
						    Tailwind's utility. It must not show over an active file tab. */}
						<div className="visible absolute right-4 top-4 z-10 rounded-md border border-solid border-border bg-surface-secondary px-3 py-2 text-xs">
							Floating chat widget
						</div>
					</div>
				</MainTabs>
			</div>
		</WorkspaceFilesContext>
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
