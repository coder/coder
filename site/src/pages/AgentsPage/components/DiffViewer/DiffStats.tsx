/**
 * Plain coloured `+N −N` counts, used in diff file headers.
 */
export const DiffStatsText: React.FC<{
	additions: number;
	deletions: number;
}> = ({ additions, deletions }) => {
	if (additions === 0 && deletions === 0) {
		return null;
	}
	return (
		<span className="flex shrink-0 gap-2 text-xs">
			{additions > 0 && (
				<span className="text-git-added-bright">+{additions}</span>
			)}
			{deletions > 0 && (
				<span className="text-git-deleted-bright">&minus;{deletions}</span>
			)}
		</span>
	);
};

/**
 * Pill-styled diff stats badge with coloured backgrounds,
 * used inside the Git tab header.
 */
export const DiffStatBadge: React.FC<{
	additions: number;
	deletions: number;
}> = ({ additions, deletions }) => {
	if (additions === 0 && deletions === 0) {
		return null;
	}
	return (
		<span className="inline-flex items-center overflow-hidden rounded-sm border font-mono text-[13px] font-medium leading-5">
			{additions > 0 && (
				<span className="flex items-center px-1.5 bg-surface-git-added text-git-added-bright">
					+{additions}
				</span>
			)}
			{deletions > 0 && (
				<span className="flex items-center px-1.5 bg-surface-git-deleted text-git-deleted-bright">
					&minus;{deletions}
				</span>
			)}
		</span>
	);
};
