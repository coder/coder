import { cn } from "cn";
import { FileIcon, FileTextIcon } from "lucide-react";
import { useState } from "react";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { isRasterImageMediaType } from "../../utils/chatAttachments";
import { ImageThumbnail } from "../AttachmentPreview";
import {
	AgentAttachmentList,
	isAgentAttachmentExpired,
} from "./AgentAttachmentList";
import { isTextPreviewAttachmentMediaType } from "./AttachmentBlocks";
import { useFileProbes } from "./FileProbeContext";
import type { AgentAttachment } from "./messageHelpers";

const THUMBNAIL_STACK_MAX = 3;

type StackSlotProps = React.PropsWithChildren<{ className?: string }>;

const StackSlot: React.FC<StackSlotProps> = ({ className, children }) => (
	<span
		className={cn(
			"inline-flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden rounded-full bg-surface-tertiary ring-1 ring-surface-primary not-first:-ml-1.5",
			className,
		)}
	>
		{children}
	</span>
);

type AttachmentThumbnailProps = {
	attachment: AgentAttachment;
	expired: boolean;
};

const AttachmentThumbnail: React.FC<AttachmentThumbnailProps> = ({
	attachment: { file, href },
	expired,
}) => {
	const [failed, setFailed] = useState(false);

	if (isRasterImageMediaType(file.media_type) && !expired && !failed) {
		return (
			<StackSlot>
				<ImageThumbnail
					previewUrl={href}
					name=""
					className="size-full rounded-none border-0"
					onError={() => setFailed(true)}
				/>
			</StackSlot>
		);
	}

	const Icon =
		!expired && isTextPreviewAttachmentMediaType(file.media_type)
			? FileTextIcon
			: FileIcon;
	return (
		<StackSlot>
			<Icon className="size-3 text-content-secondary" />
		</StackSlot>
	);
};

type TurnAttachmentsProps = {
	/** Oldest first; callers pass at least one attachment. */
	attachments: readonly AgentAttachment[];
};

/** Action bar pill for a settled turn's attachments. */
export const TurnAttachments: React.FC<TurnAttachmentsProps> = ({
	attachments,
}) => {
	const { hasExpired } = useFileProbes();
	const overflow = attachments.length - THUMBNAIL_STACK_MAX;

	return (
		<Popover>
			<PopoverTrigger asChild>
				<button
					type="button"
					aria-label={`Attachments (${attachments.length.toLocaleString("en-US")})`}
					className="inline-flex h-6 items-center gap-1.5 rounded-full border-0 bg-surface-secondary pl-0.5 pr-2 text-xs text-content-secondary transition-colors hover:bg-surface-tertiary hover:text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
				>
					<span aria-hidden className="inline-flex items-center">
						{attachments.slice(0, THUMBNAIL_STACK_MAX).map((attachment) => (
							<AttachmentThumbnail
								key={attachment.key}
								attachment={attachment}
								expired={isAgentAttachmentExpired(attachment, hasExpired)}
							/>
						))}
						{overflow > 0 && (
							<StackSlot className="w-auto min-w-5 px-0.5 text-2xs">
								+{overflow.toLocaleString("en-US")}
							</StackSlot>
						)}
					</span>
					Attachments
				</button>
			</PopoverTrigger>
			{/* Prefer opening above: below the last turn's pill sits the
			    composer, which Radix collision detection does not see. */}
			<PopoverContent
				aria-label="Attachments"
				side="top"
				align="start"
				className="w-80 max-w-[calc(100vw-2rem)] p-2"
			>
				<AgentAttachmentList attachments={attachments} isExpired={hasExpired} />
			</PopoverContent>
		</Popover>
	);
};
