import { cn } from "cn";
import { FileIcon, FileTextIcon } from "lucide-react";
import { useState } from "react";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { isRasterImageMediaType } from "../../utils/chatAttachments";
import { AgentAttachmentList } from "./AgentAttachmentList";
import { isTextPreviewAttachmentMediaType } from "./AttachmentBlocks";
import { useFileProbes } from "./FileProbeContext";
import type { AgentAttachment } from "./messageHelpers";

const THUMBNAIL_STACK_MAX = 3;

const thumbnailClassName =
	"inline-flex size-5 shrink-0 items-center justify-center overflow-hidden rounded-full bg-surface-tertiary ring-1 ring-surface-primary not-first:-ml-1.5";

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
			<span className={thumbnailClassName}>
				<img
					src={href}
					alt=""
					className="size-full object-cover"
					onError={() => setFailed(true)}
				/>
			</span>
		);
	}

	const Icon =
		!expired && isTextPreviewAttachmentMediaType(file.media_type)
			? FileTextIcon
			: FileIcon;
	return (
		<span className={thumbnailClassName}>
			<Icon className="size-3 text-content-secondary" />
		</span>
	);
};

type TurnAttachmentsProps = {
	/** The turn's attachments, oldest first. */
	attachments: readonly AgentAttachment[];
};

/** A pill previewing a turn's attachments that opens their download cards. */
export const TurnAttachments: React.FC<TurnAttachmentsProps> = ({
	attachments,
}) => {
	const { hasExpired } = useFileProbes();
	const isExpired = (file: AgentAttachment["file"]) =>
		file.file_id !== undefined && hasExpired(file.file_id);
	const overflow = attachments.length - THUMBNAIL_STACK_MAX;

	return (
		<Popover>
			<PopoverTrigger asChild>
				<button
					type="button"
					aria-label={`Attachments (${attachments.length.toLocaleString("en-US")})`}
					className="inline-flex h-6 cursor-pointer items-center gap-1.5 rounded-full border-0 bg-surface-secondary py-0 pl-0.5 pr-2 text-xs font-medium text-content-secondary transition-colors hover:bg-surface-tertiary hover:text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
				>
					<span aria-hidden className="inline-flex items-center">
						{attachments.slice(0, THUMBNAIL_STACK_MAX).map((attachment) => (
							<AttachmentThumbnail
								key={attachment.key}
								attachment={attachment}
								expired={isExpired(attachment.file)}
							/>
						))}
						{overflow > 0 && (
							<span className={cn(thumbnailClassName, "text-2xs")}>
								+{overflow.toLocaleString("en-US")}
							</span>
						)}
					</span>
					Attachments
				</button>
			</PopoverTrigger>
			<PopoverContent
				aria-label="Attachments"
				align="start"
				className="w-80 p-2"
			>
				<AgentAttachmentList attachments={attachments} isExpired={hasExpired} />
			</PopoverContent>
		</Popover>
	);
};
