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
	expiredAttachmentExplanation,
	isTextPreviewAttachmentMediaType,
} from "./AttachmentBlocks";
import { useFileProbes } from "./FileProbeContext";
import { MessageAttachmentCard } from "./MessageAttachmentCard";
import type { MessageAttachment } from "./messageHelpers";

const THUMBNAIL_STACK_MAX = 3;

type StackSlotProps = React.PropsWithChildren<{ className?: string }>;

const StackSlot: React.FC<StackSlotProps> = ({ className, children }) => (
	<span
		className={cn(
			"inline-flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden rounded-full bg-surface-secondary ring-1 ring-surface-secondary transition not-first:-ml-1.5 group-hover/attachments:bg-surface-tertiary group-hover/attachments:ring-surface-tertiary",
			className,
		)}
	>
		{children}
	</span>
);

type AttachmentThumbnailProps = {
	attachment: MessageAttachment;
};

const AttachmentThumbnail: React.FC<AttachmentThumbnailProps> = ({
	attachment: { file, href },
}) => {
	const { hasExpired } = useFileProbes();
	const [hasFailed, setHasFailed] = useState(false);

	if (
		isRasterImageMediaType(file.media_type) &&
		!hasFailed &&
		!hasExpired(file.file_id)
	) {
		return (
			<StackSlot>
				<ImageThumbnail
					previewUrl={href}
					alt=""
					className="size-full rounded-none border-0"
					loading="lazy"
					onError={() => setHasFailed(true)}
				/>
			</StackSlot>
		);
	}

	const Icon = isTextPreviewAttachmentMediaType(file.media_type)
		? FileTextIcon
		: FileIcon;
	return (
		<StackSlot>
			<Icon className="size-3" />
		</StackSlot>
	);
};

type TurnAttachmentsProps = {
	attachments: readonly MessageAttachment[];
};

export const TurnAttachments: React.FC<TurnAttachmentsProps> = ({
	attachments,
}) => {
	const { hasExpired } = useFileProbes();
	const overflow = attachments.length - THUMBNAIL_STACK_MAX;
	const hasExpiredAttachment = attachments.some(({ file }) =>
		hasExpired(file.file_id),
	);

	return (
		<Popover>
			<PopoverTrigger
				aria-label={`Attachments (${attachments.length.toLocaleString("en-US")})`}
				className="group/attachments inline-flex h-6 items-center gap-1.5 rounded-full bg-surface-secondary pl-0.5 pr-2 text-xs text-content-secondary transition-colors hover:bg-surface-tertiary hover:text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
			>
				<span aria-hidden className="inline-flex items-center">
					{attachments.slice(0, THUMBNAIL_STACK_MAX).map((attachment) => (
						<AttachmentThumbnail key={attachment.key} attachment={attachment} />
					))}
					{overflow > 0 && (
						<StackSlot className="w-auto min-w-5 px-0.5 text-2xs">
							+{overflow.toLocaleString("en-US")}
						</StackSlot>
					)}
				</span>
				Attachments
			</PopoverTrigger>
			{/* Below the last turn's pill sits the composer, which Radix
			    collision detection does not see, so opening downward
			    would cover it. */}
			<PopoverContent
				aria-label="Attachments"
				side="top"
				align="start"
				className="w-80 max-w-(--radix-popper-available-width) p-2"
			>
				<ul className="flex flex-col gap-2">
					{attachments.map((attachment) => (
						<li key={attachment.key}>
							<MessageAttachmentCard attachment={attachment} />
						</li>
					))}
				</ul>
				{hasExpiredAttachment && (
					<p className="mt-2 text-xs text-content-secondary">
						{expiredAttachmentExplanation}
					</p>
				)}
			</PopoverContent>
		</Popover>
	);
};
