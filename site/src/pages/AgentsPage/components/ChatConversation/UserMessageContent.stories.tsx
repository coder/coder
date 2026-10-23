import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { WorkspaceFilesContext } from "../../context/WorkspaceFilesContext";
import type { MessageDisplayState } from "./messageHelpers";
import { UserMessageContent } from "./UserMessageContent";

const messageText =
	"compare `/home/coder/coder/cmd/coder/main.go` with `/home/coder/coder/site/src/` and `/home/coder/coder/docs/Release Notes.md` but keep `make gen` as is";

const displayState: MessageDisplayState = {
	shouldHide: false,
	userInlineContent: [{ type: "response", text: messageText }],
	userFileBlocks: [],
	workspaceFileBlocks: [],
	workspaceFileReferenceCount: 0,
	hasUserMessageBody: true,
	hasFileBlocks: false,
	hasWorkspaceFileReferences: false,
	hasCopyableContent: true,
	needsAssistantBottomSpacer: false,
};

const meta: Meta<typeof UserMessageContent> = {
	title: "pages/AgentsPage/ChatConversation/UserMessageContent",
	component: UserMessageContent,
	args: {
		displayState,
		markdown: messageText,
		isEditing: false,
		onImageClick: fn(),
		onTextFileClick: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof UserMessageContent>;

export const WorkspacePathReferences: Story = {
	decorators: [
		(Story) => (
			<WorkspaceFilesContext
				value={{
					agentId: "agent-1",
					defaultDirectory: "/home/coder",
					repoRoots: ["/home/coder/coder"],
					repoRootsSettled: true,
					repositories: new Map(),
					openFile: fn(),
					revealInFiles: fn(),
					diffStatuses: [],
				}}
			>
				<Story />
			</WorkspaceFilesContext>
		),
	],
};

export const WorkspacePathReferencesWithoutWorkspace: Story = {};
