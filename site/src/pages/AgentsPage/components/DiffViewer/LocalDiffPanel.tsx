import { parsePatchFiles } from "@pierre/diffs";
import type { WorkspaceAgentRepoChanges } from "api/typesGenerated";
import type { FC, RefObject } from "react";
import type { ChatMessageInputRef } from "../AgentChatInput";
import { CommentableDiffViewer } from "../DiffViewer/CommentableDiffViewer";
import type { DiffStyle } from "../DiffViewer/DiffViewer";

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
	const parsedFiles = (() => {
		const diff = repo.unified_diff;
		if (!diff) {
			return [];
		}
		try {
			const patches = parsePatchFiles(diff);
			return patches.flatMap((p) => p.files);
		} catch {
			return [];
		}
	})();

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
