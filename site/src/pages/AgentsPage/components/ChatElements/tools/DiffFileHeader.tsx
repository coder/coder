import type { FileContents, FileDiffMetadata } from "@pierre/diffs";
import { cn } from "#/utils/cn";
import { countChangedLines } from "../../../utils/countChangedLines";
import { changeColor, changeLabel } from "../../../utils/diffColors";

export function DiffFileHeader({
	file,
}: {
	file: FileDiffMetadata | FileContents;
}) {
	const isDiff = "type" in file;
	const stats = isDiff ? countChangedLines(file) : null;

	return (
		<div className="flex h-8 min-w-0 items-center justify-between gap-3 border-0 border-b border-l border-solid border-border-default bg-transparent py-2 pr-1.5 pl-2.5 font-sans text-sm">
			<div className="flex min-w-0 items-baseline gap-2 overflow-hidden">
				{isDiff && (
					<span
						className={cn(
							"shrink-0 text-[11px] font-semibold leading-none",
							changeColor(file.type),
						)}
					>
						{changeLabel(file.type)}
					</span>
				)}
				{isDiff && file.prevName && file.prevName !== file.name && (
					<span className="truncate text-xs text-content-secondary">
						{file.prevName}
					</span>
				)}
				<span className="truncate text-xs font-medium text-content-primary">
					{file.name}
				</span>
			</div>
			{stats && (stats.additions > 0 || stats.deletions > 0) && (
				<span className="inline-flex shrink-0 flex-row-reverse items-stretch overflow-hidden rounded-[3px] border border-solid border-border-default font-mono text-xs font-medium leading-5">
					{stats.deletions > 0 && (
						<span className="flex items-center bg-surface-git-deleted px-1 text-git-deleted-bright">
							&minus;{stats.deletions}
						</span>
					)}
					{stats.additions > 0 && (
						<span className="flex items-center bg-surface-git-added px-1 text-git-added-bright">
							+{stats.additions}
						</span>
					)}
				</span>
			)}
		</div>
	);
}
