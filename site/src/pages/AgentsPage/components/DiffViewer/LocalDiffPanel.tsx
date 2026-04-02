import { parsePatchFiles } from "@pierre/diffs";
import type { FC, RefObject } from "react";
import { API } from "#/api/api";
import type { WorkspaceAgentRepoChanges } from "#/api/typesGenerated";
import { extractFilePatch } from "../../utils/extractFilePatch";
import type { ChatMessageInputRef } from "../AgentChatInput";
import { CommentableDiffViewer } from "../DiffViewer/CommentableDiffViewer";
import type { DiffStyle } from "../DiffViewer/DiffViewer";

interface LocalDiffPanelProps {
	chatId: string;
	repo: WorkspaceAgentRepoChanges;
	isExpanded?: boolean;
	diffStyle: DiffStyle;
	chatInputRef?: RefObject<ChatMessageInputRef | null>;
}

export const LocalDiffPanel: FC<LocalDiffPanelProps> = ({
	chatId,
	repo,
	isExpanded,
	diffStyle,
	chatInputRef,
}) => {
	const diff = repo.unified_diff ?? "";

	const parsedFiles = (() => {
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

	const handleRequestFileContents = diff
		? async (fileName: string) => {
				const patchString = extractFilePatch(diff, fileName);
				if (!patchString) return null;

				try {
					const [oldResult, newResult] = await Promise.allSettled([
						API.experimental.getChatFileContent(
							chatId,
							repo.repo_root,
							fileName,
							"old",
						),
						API.experimental.getChatFileContent(
							chatId,
							repo.repo_root,
							fileName,
							"new",
						),
					]);

					const oldContents =
						oldResult.status === "fulfilled"
							? oldResult.value.contents
							: null;
					const newContents =
						newResult.status === "fulfilled"
							? newResult.value.contents
							: null;

					return { oldContents, newContents, patchString };
				} catch {
					return { oldContents: null, newContents: null, patchString };
				}
			}
		: undefined;

	return (
		<CommentableDiffViewer
			parsedFiles={parsedFiles}
			isExpanded={isExpanded}
			emptyMessage="No file changes."
			diffStyle={diffStyle}
			chatInputRef={chatInputRef}
			onRequestFileContents={handleRequestFileContents}
		/>
	);
};
