import type { FileContents, FileDiffMetadata } from "@pierre/diffs";
import { cn } from "cn";
import { countChangedLines } from "../../../utils/countChangedLines";
import { changeColor, changeLabel } from "../../../utils/diffColors";
import { DiffStatsText } from "../../DiffViewer/DiffStats";
import { OpenFileButton } from "./FileToolHeader";

export function DiffFileHeader({
	file,
	openPath,
}: {
	file: FileDiffMetadata | FileContents;
	/**
	 * Absolute path that a button in the header opens, for rows that list
	 * several files. Diff parsing can drop the leading slash from the file
	 * name, so the caller passes the path it acted on.
	 */
	openPath?: string;
}) {
	const isDiff = "type" in file;
	const stats = isDiff ? countChangedLines(file) : null;

	return (
		<div className="flex h-8 min-w-0 items-center justify-between gap-3 border-b bg-surface-secondary py-2 pr-1.5 pl-2.5 font-sans text-sm">
			<div className="flex min-w-0 flex-1 items-baseline gap-2 truncate">
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
				<span className="truncate text-xs text-content-primary">
					{file.name}
				</span>
			</div>
			<div className="flex shrink-0 items-center gap-1">
				{stats && (
					<DiffStatsText
						additions={stats.additions}
						deletions={stats.deletions}
					/>
				)}
				{openPath && <OpenFileButton path={openPath} />}
			</div>
		</div>
	);
}
