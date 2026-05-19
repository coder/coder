import { CheckIcon, CopyIcon, HardDriveIcon, XIcon } from "lucide-react";
import type { FC } from "react";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useClipboard } from "#/hooks/useClipboard";
import { cn } from "#/utils/cn";

/**
 * Formats a byte count using IEC binary prefixes (KiB, MiB, GiB).
 */
const formatWorkspaceFileSize = (bytes: number): string => {
	if (bytes < 1024) return `${bytes} B`;
	const units = ["KiB", "MiB", "GiB", "TiB"];
	let n = bytes / 1024;
	let i = 0;
	while (n >= 1024 && i < units.length - 1) {
		n /= 1024;
		i++;
	}
	const rounded = n >= 10 ? Math.round(n).toString() : n.toFixed(1);
	return `${rounded} ${units[i]}`;
};

interface WorkspaceFileChipProps {
	name: string;
	size: number;
	path?: string;
	isUploading?: boolean;
	errorMessage?: string;
	onRemove?: () => void;
	className?: string;
}

/**
 * Renders a workspace-file attachment as a pill: filename, size, and an
 * optional copy-path action. Used in the chat input draft strip and in
 * the sent user message body.
 */
export const WorkspaceFileChip: FC<WorkspaceFileChipProps> = ({
	name,
	size,
	path,
	isUploading,
	errorMessage,
	onRemove,
	className,
}) => {
	const clipboard = useClipboard();
	const hasError = Boolean(errorMessage);

	return (
		<span
			className={cn(
				"group inline-flex max-w-[min(100%,360px)] items-center gap-1.5 rounded-md border border-border-default bg-surface-primary px-2 py-1 text-xs text-content-primary shadow-sm",
				hasError && "border-content-destructive/40 bg-content-destructive/5",
				className,
			)}
			title={errorMessage ?? path ?? name}
		>
			{isUploading ? (
				<Spinner loading className="size-3.5 shrink-0" />
			) : (
				<HardDriveIcon
					className={cn(
						"size-3.5 shrink-0 text-content-secondary",
						hasError && "text-content-destructive",
					)}
					aria-hidden
				/>
			)}
			<span className="flex min-w-0 flex-col leading-tight">
				<span className="truncate font-medium" title={name}>
					{name}
				</span>
				<span
					className={cn(
						"truncate text-[10px] text-content-secondary",
						hasError && "text-content-destructive",
					)}
				>
					{hasError
						? errorMessage
						: isUploading
							? `Uploading… ${formatWorkspaceFileSize(size)}`
							: `${formatWorkspaceFileSize(size)} · workspace`}
				</span>
			</span>
			{path && !isUploading && !hasError && (
				<Tooltip>
					<TooltipTrigger asChild>
						<button
							type="button"
							onClick={() => {
								void clipboard.copyToClipboard(path);
							}}
							aria-label={
								clipboard.showCopiedSuccess
									? "Copied path"
									: `Copy workspace path for ${name}`
							}
							className="ml-0.5 inline-flex shrink-0 items-center justify-center rounded p-0.5 text-content-secondary hover:bg-surface-tertiary hover:text-content-primary"
						>
							{clipboard.showCopiedSuccess ? (
								<CheckIcon className="size-3" />
							) : (
								<CopyIcon className="size-3" />
							)}
						</button>
					</TooltipTrigger>
					<TooltipContent>
						{clipboard.showCopiedSuccess ? "Copied" : "Copy workspace path"}
					</TooltipContent>
				</Tooltip>
			)}
			{onRemove && (
				<button
					type="button"
					onClick={onRemove}
					aria-label={`Remove ${name}`}
					className="ml-0.5 inline-flex shrink-0 items-center justify-center rounded p-0.5 text-content-secondary hover:bg-surface-tertiary hover:text-content-primary"
				>
					<XIcon className="size-3" />
				</button>
			)}
		</span>
	);
};
