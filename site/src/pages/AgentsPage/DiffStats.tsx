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
 * Always renders both counters so that zero-line changes (e.g.
 * binary files like images) still display "+0 −0".
 */
const DiffStatNumbers: FC<DiffStatsProps> = ({
	additions,
	deletions,
	className,
}) => {
	return (
		<span
			className={cn(
				"inline-flex items-center gap-0.5 font-mono text-xs font-medium tabular-nums",
				className,
			)}
		>
			<span className="text-green-700 dark:text-green-500">+{additions}</span>
			<span className="text-red-700 dark:text-red-400">&minus;{deletions}</span>
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
	const hasChangedFiles = (status.changed_files ?? 0) > 0;

	if (!hasChangedFiles && additions === 0 && deletions === 0) {
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
