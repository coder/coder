import type { FC } from "react";

/**
 * Pill-styled diff stats badge with coloured backgrounds,
 * used inside the Git tab header.
 */
export const DiffStatBadge: FC<{ additions: number; deletions: number }> = ({
	additions,
	deletions,
}) => {
	if (additions === 0 && deletions === 0) {
		return null;
	}
	return (
		<span className="inline-flex h-full items-center self-stretch overflow-hidden font-mono text-xs font-medium">
			{additions > 0 && (
				<span className="flex h-full items-center bg-surface-git-added px-1.5 text-git-added-bright">
					+{additions}
				</span>
			)}
			{deletions > 0 && (
				<span className="flex h-full items-center bg-surface-git-deleted px-1.5 text-git-deleted-bright">
					&minus;{deletions}
				</span>
			)}
		</span>
	);
};
