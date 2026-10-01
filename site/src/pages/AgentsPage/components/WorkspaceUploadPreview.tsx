import { cn } from "cn";
import { HardDriveUploadIcon, XIcon } from "lucide-react";
import prettyBytes from "pretty-bytes";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import {
	isWorkspaceUploadInProgress,
	type WorkspaceFileUpload,
} from "../hooks/useWorkspaceFileUploads";
import { attachmentActionsLockedMessage } from "./AttachmentPreview";

const uploadStatusLabel = (upload: WorkspaceFileUpload): string => {
	switch (upload.status) {
		case "deferred":
			return "Uploads when sent";
		case "queued":
			return "Waiting to upload...";
		case "uploading":
			return "Uploading to workspace...";
		case "uploaded":
			return upload.response
				? `${prettyBytes(upload.response.size)} in workspace`
				: "In workspace";
		case "error":
			return upload.error ?? "Upload failed";
	}
};

/**
 * Composer chip strip for files streamed into the chat's workspace
 * filesystem. Uploads are eager, so removing a chip only detaches the
 * reference from the draft message; bytes already written stay in the
 * workspace.
 */
export const WorkspaceUploadPreview: React.FC<{
	uploads: readonly WorkspaceFileUpload[];
	onRemove: (id: string) => void;
	removeDisabled: boolean;
}> = ({ uploads, onRemove, removeDisabled }) => {
	if (uploads.length === 0) {
		return null;
	}
	return (
		<div className="flex flex-wrap gap-2 px-3 pt-3">
			{uploads.map((upload) => {
				const name = upload.response?.name ?? upload.file.name;
				return (
					<div
						key={upload.id}
						className="flex max-w-64 items-center gap-2 rounded-md border border-solid border-border-default bg-surface-tertiary px-2.5 py-1.5 text-xs"
					>
						{upload.status === "uploading" ? (
							<Spinner
								className="size-3.5 shrink-0 text-content-secondary"
								loading
							/>
						) : (
							<HardDriveUploadIcon
								aria-hidden="true"
								className="size-3.5 shrink-0 text-content-secondary"
							/>
						)}
						<div className="min-w-0 flex-1">
							<div className="truncate font-medium text-content-primary">
								{name}
							</div>
							<div
								className={cn(
									"truncate text-2xs",
									upload.status === "error"
										? "text-content-destructive"
										: "text-content-secondary",
								)}
							>
								{uploadStatusLabel(upload)}
							</div>
						</div>
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									type="button"
									variant="subtle"
									size="icon"
									className="size-5 shrink-0 text-content-secondary hover:text-content-primary aria-disabled:cursor-not-allowed aria-disabled:text-content-disabled aria-disabled:hover:text-content-disabled"
									aria-label={`Remove ${name}`}
									aria-disabled={removeDisabled}
									onClick={() => {
										if (removeDisabled) return;
										onRemove(upload.id);
									}}
								>
									<XIcon className="size-3.5" />
								</Button>
							</TooltipTrigger>
							<TooltipContent side="top">
								{removeDisabled
									? attachmentActionsLockedMessage
									: upload.status === "uploaded"
										? "Removes the reference. Uploaded bytes stay in the workspace."
										: isWorkspaceUploadInProgress(upload)
											? "Cancel this upload"
											: "Remove this file"}
							</TooltipContent>
						</Tooltip>
					</div>
				);
			})}
		</div>
	);
};
