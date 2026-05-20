import {
	AlertTriangleIcon,
	CheckIcon,
	CopyIcon,
	DownloadIcon,
	FileIcon,
	XIcon,
} from "lucide-react";
import prettyBytes from "pretty-bytes";
import type { FC, MouseEvent, ReactNode } from "react";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { useClipboard } from "#/hooks/useClipboard";
import { cn } from "#/utils/cn";
import { getFileAttachmentBadgeLabel } from "../utils/fileAttachmentUtils";

const formatFileSize = (bytes: number | undefined): string | undefined => {
	if (bytes === undefined || !Number.isFinite(bytes) || bytes < 0) {
		return undefined;
	}
	return prettyBytes(bytes, { binary: true });
};

interface FileAttachmentTileProps {
	name: string;
	size?: number;
	mediaType?: string;
	metadataLabel?: string;
	preview?: ReactNode;
	onClick?: () => void | Promise<void>;
	clickLabel?: string;
	href?: string | null;
	downloadName?: string;
	copyPath?: string;
	onRemove?: () => void;
	isUploading?: boolean;
	isLoading?: boolean;
	errorMessage?: string;
	extraActions?: ReactNode;
	className?: string;
	title?: string;
}

export const FileAttachmentTile: FC<FileAttachmentTileProps> = ({
	name,
	size,
	mediaType,
	metadataLabel,
	preview,
	onClick,
	clickLabel,
	href,
	downloadName,
	copyPath,
	onRemove,
	isUploading,
	isLoading,
	errorMessage,
	extraActions,
	className,
	title,
}) => {
	const clipboard = useClipboard();
	const hasError = Boolean(errorMessage);
	const isBusy = Boolean(isUploading || isLoading);
	const formattedSize = formatFileSize(size);
	const badgeLabel = getFileAttachmentBadgeLabel({ mediaType, name });
	const secondaryParts = [formattedSize, metadataLabel].filter(Boolean);
	const secondary = hasError
		? errorMessage
		: isUploading
			? ["Uploading", formattedSize].filter(Boolean).join(" ")
			: secondaryParts.join(" · ");

	const stopAction = (event: MouseEvent) => {
		event.stopPropagation();
	};

	const visual = preview ?? (
		<div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-surface-secondary">
			{badgeLabel ? (
				<span className="text-[10px] font-semibold tracking-wide text-content-secondary">
					{badgeLabel}
				</span>
			) : (
				<FileIcon
					aria-hidden="true"
					className="h-4 w-4 text-content-secondary"
				/>
			)}
		</div>
	);

	const content = (
		<>
			<div className="relative flex h-10 w-10 shrink-0 items-center justify-center">
				{visual}
				{isBusy && (
					<div className="absolute inset-0 flex items-center justify-center rounded-md bg-overlay">
						<Spinner className="h-5 w-5 text-white" loading />
					</div>
				)}
				{hasError && (
					<Tooltip>
						<TooltipTrigger asChild>
							<div
								className="absolute inset-0 flex items-center justify-center rounded-md bg-overlay"
								role="img"
								aria-label="Upload error"
							>
								<AlertTriangleIcon className="h-5 w-5 text-content-warning" />
							</div>
						</TooltipTrigger>
						<TooltipContent side="top">
							<p className="max-w-xs text-xs">{errorMessage}</p>
						</TooltipContent>
					</Tooltip>
				)}
			</div>
			<span className="min-w-0 flex-1 text-left leading-tight">
				<span className="block truncate text-sm font-medium text-content-primary">
					{name}
				</span>
				{secondary && (
					<span
						className={cn(
							"block truncate text-xs text-content-secondary",
							hasError && "text-content-destructive",
						)}
					>
						{secondary}
					</span>
				)}
			</span>
		</>
	);

	const bodyClassName = cn(
		"group/attachment inline-flex h-16 max-w-sm items-center gap-3 rounded-md border border-solid border-border-default bg-surface-tertiary px-3 py-2 text-content-primary shadow-sm transition-colors",
		onClick &&
			"cursor-pointer hover:bg-surface-quaternary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
		hasError && "border-content-destructive/40 bg-content-destructive/5",
		className,
	);

	const body = onClick ? (
		<button
			type="button"
			aria-label={clickLabel ?? `View ${name}`}
			className={bodyClassName}
			title={title ?? errorMessage ?? copyPath ?? name}
			onClick={(event) => {
				event.stopPropagation();
				void onClick();
			}}
		>
			{content}
		</button>
	) : (
		<div
			className={bodyClassName}
			title={title ?? errorMessage ?? copyPath ?? name}
		>
			{content}
		</div>
	);

	return (
		<div className="group relative inline-flex max-w-full items-center">
			{body}
			<div
				className={cn(
					"ml-1 inline-flex shrink-0 items-center gap-0.5",
					onClick &&
						"invisible opacity-0 transition-opacity group-hover:visible group-hover:opacity-100 group-focus-within:visible group-focus-within:opacity-100 [@media(hover:none)]:visible [@media(hover:none)]:opacity-100",
				)}
			>
				{extraActions}
				{href && !hasError && (
					<Tooltip>
						<TooltipTrigger asChild>
							<a
								href={href}
								download={downloadName}
								onClick={stopAction}
								aria-label={`Download ${name}`}
								className="inline-flex size-6 items-center justify-center rounded text-content-secondary hover:bg-surface-tertiary hover:text-content-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link"
							>
								<DownloadIcon aria-hidden="true" className="size-3.5" />
							</a>
						</TooltipTrigger>
						<TooltipContent>Download file</TooltipContent>
					</Tooltip>
				)}
				{copyPath && !isBusy && !hasError && (
					<Tooltip>
						<TooltipTrigger asChild>
							<button
								type="button"
								onClick={(event) => {
									stopAction(event);
									void clipboard.copyToClipboard(copyPath);
								}}
								aria-label={
									clipboard.showCopiedSuccess
										? "Copied path"
										: `Copy workspace path for ${name}`
								}
								className="inline-flex size-6 items-center justify-center rounded text-content-secondary hover:bg-surface-tertiary hover:text-content-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link"
							>
								{clipboard.showCopiedSuccess ? (
									<CheckIcon aria-hidden="true" className="size-3.5" />
								) : (
									<CopyIcon aria-hidden="true" className="size-3.5" />
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
						onClick={(event) => {
							stopAction(event);
							onRemove();
						}}
						aria-label={`Remove ${name}`}
						className="inline-flex size-6 items-center justify-center rounded text-content-secondary hover:bg-surface-tertiary hover:text-content-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link"
					>
						<XIcon aria-hidden="true" className="size-3.5" />
					</button>
				)}
			</div>
		</div>
	);
};
