import type { FC } from "react";
import type { ChatContextResourceChange } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Spinner } from "#/components/Spinner/Spinner";
import { cn } from "#/utils/cn";
import { buildContentPatch } from "./ChatElements/tools/utils";
import { DiffViewer } from "./DiffViewer/DiffViewer";
import { useParsedDiff } from "./DiffViewer/useParsedDiff";

interface ContextChangesDialogProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	changes: readonly ChatContextResourceChange[];
	onRefreshContext?: () => void;
	isRefreshingContext?: boolean;
}

const CHANGE_LABELS: Record<
	ChatContextResourceChange["status"],
	{ readonly letter: string; readonly text: string; readonly className: string }
> = {
	added: { letter: "A", text: "Added", className: "text-git-added" },
	removed: { letter: "D", text: "Removed", className: "text-git-deleted" },
	modified: { letter: "M", text: "Modified", className: "text-git-modified" },
};

/**
 * Renders the differences between a chat's pinned workspace context and the
 * agent's latest snapshot. Instruction-file edits are shown as a diff (reusing
 * the chat DiffViewer); skill changes are shown as a labeled list since they
 * carry only a name and description. A refresh action re-pins to the latest
 * snapshot.
 */
export const ContextChangesDialog: FC<ContextChangesDialogProps> = ({
	open,
	onOpenChange,
	changes,
	onRefreshContext,
	isRefreshingContext,
}) => {
	const fileChanges = changes.filter(
		(change) => change.kind === "instruction_file",
	);
	const skillChanges = changes.filter((change) => change.kind === "skill");

	// Concatenate per-file patches into one diff string and let useParsedDiff
	// perform the (memoized) parse, rather than parsing in render.
	const diffString = fileChanges
		.map((change) =>
			buildContentPatch(
				change.source,
				change.old_content ?? "",
				change.new_content ?? "",
			),
		)
		.join("");
	const parsedFiles = useParsedDiff(diffString || null, "context-changes");

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="flex max-h-[80vh] w-full max-w-4xl flex-col gap-4 p-6">
				<DialogHeader className="space-y-1">
					<DialogTitle>Context changes</DialogTitle>
					<DialogDescription>
						How this chat's pinned context differs from the workspace's latest
						context. Refresh to use the latest.
					</DialogDescription>
				</DialogHeader>

				{skillChanges.length > 0 && (
					<ul className="m-0 flex list-none flex-col gap-1 p-0 text-sm">
						{skillChanges.map((change) => {
							const label = CHANGE_LABELS[change.status];
							return (
								<li
									key={change.source}
									className="flex items-center gap-2 text-content-secondary"
								>
									<span
										className={cn("w-4 shrink-0 font-mono", label.className)}
										title={label.text}
									>
										{label.letter}
									</span>
									<span className="font-medium text-content-primary">
										{change.skill_name || change.source}
									</span>
									{change.skill_description && (
										<span className="truncate">{change.skill_description}</span>
									)}
								</li>
							);
						})}
					</ul>
				)}

				{parsedFiles.length > 0 && (
					<div className="min-h-0 flex-1 overflow-hidden rounded-md border border-solid border-border-default">
						<DiffViewer
							parsedFiles={parsedFiles}
							diffStyle="unified"
							emptyMessage="No file changes."
						/>
					</div>
				)}

				<div className="flex justify-end gap-2">
					<Button variant="outline" onClick={() => onOpenChange(false)}>
						Close
					</Button>
					{onRefreshContext && (
						<Button
							disabled={isRefreshingContext}
							onClick={() => onRefreshContext()}
						>
							<Spinner loading={isRefreshingContext} />
							Refresh context
						</Button>
					)}
				</div>
			</DialogContent>
		</Dialog>
	);
};
