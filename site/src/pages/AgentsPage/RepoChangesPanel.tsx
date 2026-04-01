import { parsePatchFiles } from "@pierre/diffs";
import type { WorkspaceAgentRepoChanges } from "api/typesGenerated";
import { type FC, useMemo } from "react";
import { type DiffStyle, DiffViewer } from "./DiffViewer";

interface RepoChangesPanelProps {
	repo: WorkspaceAgentRepoChanges;
	isExpanded?: boolean;
	diffStyle: DiffStyle;
}

export const RepoChangesPanel: FC<RepoChangesPanelProps> = ({
	repo,
	isExpanded,
	diffStyle,
}) => {
	const parsedFiles = useMemo(() => {
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
	}, [repo.unified_diff]);

	return (
		<DiffViewer
			parsedFiles={parsedFiles}
			isExpanded={isExpanded}
			emptyMessage="No file changes."
			diffStyle={diffStyle}
		/>
	);
};
