import type { FileDiffMetadata } from "@pierre/diffs";
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
	const { parsedFiles, parseError } = (() => {
		const diff = repo.unified_diff;
		if (!diff) {
			return { parsedFiles: [] as FileDiffMetadata[], parseError: undefined };
		}
		try {
			const patches = parsePatchFiles(diff);
			return {
				parsedFiles: patches.flatMap((p) => p.files),
				parseError: undefined,
			};
		} catch (e) {
			return { parsedFiles: [] as FileDiffMetadata[], parseError: e };
		}
	})();

	return (
		<CommentableDiffViewer
			parsedFiles={parsedFiles}
			isExpanded={isExpanded}
			emptyMessage="No file changes."
			diffStyle={diffStyle}
			chatInputRef={chatInputRef}
			error={
				parseError
					? new Error(
							"Failed to parse local diff data. The diff output may be malformed.",
						)
					: undefined
			}
		/>
	);
};
