import type { FC, RefObject } from "react";
import type { WorkspaceAgentRepoChanges } from "#/api/typesGenerated";
import type { ChatMessageInputRef } from "../AgentChatInput";
import { CommentableDiffViewer } from "../DiffViewer/CommentableDiffViewer";
import type { DiffStyle } from "../DiffViewer/DiffViewer";
import { useParsedDiff } from "../DiffViewer/useParsedDiff";

interface LocalDiffPanelProps {
	repo: WorkspaceAgentRepoChanges;
	isExpanded?: boolean;
	diffStyle: DiffStyle;
	chatInputRef?: RefObject<ChatMessageInputRef | null>;
}

export const LocalDiffPanel: FC<LocalDiffPanelProps> = ({
	repo,
	isExpanded,
	diffStyle,
	chatInputRef,
}) => {
	const parsedFiles = useParsedDiff(repo.unified_diff);

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
