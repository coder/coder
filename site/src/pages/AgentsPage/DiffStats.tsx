import type { ChatDiffStatusResponse } from "api/api";
import type { FC } from "react";
import { cn } from "utils/cn";

interface DiffStatsProps {
	additions: number;
	deletions: number;
	className?: string;
}

/**
 * Renders +N / −N counters for diff additions and deletions.
 * Returns null when both values are zero.
 */
const DiffStatNumbers: FC<DiffStatsProps> = ({
	additions,
	deletions,
	className,
}) => {
	if (additions === 0 && deletions === 0) {
		return null;
	}
	return (
		<span
			className={cn(
				"inline-flex items-center gap-0.5 font-mono text-xs font-medium tabular-nums",
				className,
			)}
		>
			{additions > 0 && (
				<span className="text-green-700 dark:text-green-500">+{additions}</span>
			)}
			{deletions > 0 && (
				<span className="text-red-700 dark:text-red-400">
					&minus;{deletions}
				</span>
			)}
		</span>
	);
};

/**
 * Pill-styled diff stats badge with coloured backgrounds,
 * used inside the Git tab header.
 */
export const DiffStatBadge: FC<{ diffStatus?: ChatDiffStatusResponse }> = ({
	diffStatus,
}) => {
	const additions = diffStatus?.additions ?? 0;
	const deletions = diffStatus?.deletions ?? 0;
	if (additions === 0 && deletions === 0) {
		return null;
	}
	return (
		<span className="inline-flex h-full items-center self-stretch overflow-hidden rounded-[calc(theme(borderRadius.md)-1px)] font-mono text-xs font-medium">
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

/**
 * Clickable inline diff stats shown in the top bar when the
 * diff panel is closed.
 */
export const DiffStatsInline: FC<{
	status: ChatDiffStatusResponse;
	onClick: () => void;
}> = ({ status, onClick }) => {
	const additions = status.additions ?? 0;
	const deletions = status.deletions ?? 0;

	if (additions === 0 && deletions === 0) {
		return null;
	}

	return (
		<button
			type="button"
			onClick={onClick}
			className="inline-flex shrink-0 cursor-pointer items-center border-0 bg-transparent p-0 leading-none transition-opacity hover:opacity-80 outline-none"
		>
			<DiffStatNumbers additions={additions} deletions={deletions} />
		</button>
	);
};
