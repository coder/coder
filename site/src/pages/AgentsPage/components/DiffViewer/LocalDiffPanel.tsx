import type { WorkspaceAgentRepoChanges } from "#/api/typesGenerated";
import type { ChatMessageInputRef } from "../AgentChatInput";
import { CommentableDiffViewer } from "../DiffViewer/CommentableDiffViewer";
import type { DiffStyle } from "../DiffViewer/DiffViewer";
import { parseDiffString } from "../DiffViewer/parseDiff";

type LocalDiffPanelProps = {
	repo: WorkspaceAgentRepoChanges;
	isExpanded?: boolean;
	diffStyle: DiffStyle;
	chatInputRef?: React.RefObject<ChatMessageInputRef | null>;
};

export const LocalDiffPanel: React.FC<LocalDiffPanelProps> = ({
	repo,
	isExpanded,
	diffStyle,
	chatInputRef,
}) => {
	const parsedFiles = parseDiffString(repo.unified_diff);

	return (
		<CommentableDiffViewer
			parsedFiles={parsedFiles}
			isExpanded={isExpanded}
			emptyMessage="No file changes."
			diffStyle={diffStyle}
			chatInputRef={chatInputRef}
		/>
	);
};
