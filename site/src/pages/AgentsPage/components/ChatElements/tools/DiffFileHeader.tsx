import type { FileContents, FileDiffMetadata } from "@pierre/diffs";
import { cn } from "cn";
import { useWorkspaceFiles } from "../../../context/WorkspaceFilesContext";
import { countChangedLines } from "../../../utils/countChangedLines";
import { changeColor, changeLabel } from "../../../utils/diffColors";
import { DiffStatsText } from "../../DiffViewer/DiffStats";

export function DiffFileHeader({
	file,
}: {
	file: FileDiffMetadata | FileContents;
}) {
	const isDiff = "type" in file;
	const stats = isDiff ? countChangedLines(file) : null;
	const files = useWorkspaceFiles();

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
				{files && file.name.startsWith("/") ? (
					<button
						type="button"
						aria-label={`Open ${file.name}`}
						className="m-0 min-w-0 cursor-pointer truncate border-0 bg-transparent p-0 text-left font-[inherit] text-xs text-content-primary underline decoration-dotted underline-offset-2 hover:text-content-link"
						onClick={() => files.openFile(file.name)}
					>
						{file.name}
					</button>
				) : (
					<span className="truncate text-xs text-content-primary">
						{file.name}
					</span>
				)}
			</div>
			{stats && (
				<DiffStatsText
					additions={stats.additions}
					deletions={stats.deletions}
				/>
			)}
		</div>
	);
}
