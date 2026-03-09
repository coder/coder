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
				<span className="flex h-full items-center bg-green-100 dark:bg-green-950 px-1.5 text-green-700 dark:text-green-500">
					+{additions}
				</span>
			)}
			{deletions > 0 && (
				<span className="flex h-full items-center bg-red-100 dark:bg-red-950 px-1.5 text-red-700 dark:text-red-400">
					&minus;{deletions}
				</span>
			)}
		</span>
	);
};
