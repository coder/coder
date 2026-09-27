import { cn } from "cn";
import { HardDriveUploadIcon, RotateCcwIcon, XIcon } from "lucide-react";
import prettyBytes from "pretty-bytes";
import type { FC } from "react";
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

export const parkedWorkspaceUploadLabel = "Uploads when the workspace starts";

const uploadStatusLabel = (
	upload: WorkspaceFileUpload,
	deferredLabel: string,
): string => {
	switch (upload.status) {
		case "deferred":
			return deferredLabel;
		case "queued":
			return "Waiting to upload...";
		case "uploading":
			return upload.progress === undefined
				? "Uploading to workspace..."
				: `Uploading to workspace... ${upload.progress}%`;
		case "uploaded":
			return upload.response
				? `${prettyBytes(upload.response.size)} in workspace`
				: "In workspace";
		case "error":
			return upload.error ?? "Upload failed";
	}
};

type WorkspaceUploadPreviewProps = {
	uploads: readonly WorkspaceFileUpload[];
	onRemove: (id: string) => void;
	onRetry?: (id: string) => void;
	deferredLabel?: string;
};

/**
 * Composer chip strip for files streamed into the chat's workspace
 * filesystem. Uploads are eager, so removing a chip only detaches the
 * reference from the draft message; bytes already written stay in the
 * workspace.
 */
export const WorkspaceUploadPreview: FC<WorkspaceUploadPreviewProps> = ({
	uploads,
	onRemove,
	onRetry,
	deferredLabel = "Uploads when sent",
}) => {
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
								{uploadStatusLabel(upload, deferredLabel)}
							</div>
						</div>
						{onRetry && upload.status === "error" && (
							<Tooltip>
								<TooltipTrigger asChild>
									<Button
										type="button"
										variant="subtle"
										size="icon"
										className="size-5 shrink-0 text-content-secondary hover:text-content-primary"
										aria-label={`Retry uploading ${name}`}
										onClick={() => onRetry(upload.id)}
									>
										<RotateCcwIcon className="size-3.5" />
									</Button>
								</TooltipTrigger>
								<TooltipContent side="top">Retry this upload</TooltipContent>
							</Tooltip>
						)}
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									type="button"
									variant="subtle"
									size="icon"
									className="size-5 shrink-0 text-content-secondary hover:text-content-primary"
									aria-label={`Remove ${name}`}
									onClick={() => onRemove(upload.id)}
								>
									<XIcon className="size-3.5" />
								</Button>
							</TooltipTrigger>
							<TooltipContent side="top">
								{upload.status === "uploaded"
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
