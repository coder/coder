import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { Spinner } from "#/components/Spinner/Spinner";
import { FileCard } from "./ChatConversation/AttachmentBlocks";
import type { MessageAttachment } from "./ChatConversation/messageHelpers";
import { RetryButton } from "./RetryButton";
import { SectionHeader } from "./SectionHeader";

type ChatAttachmentsProps = {
	/** Undefined while the list loads. */
	attachments: readonly MessageAttachment[] | undefined;
	evictedFileIds: ReadonlySet<string>;
	hasEarlierAttachmentsError: boolean;
	isRetrying: boolean;
	onRetry: () => void;
};

/** Collapsible download cards for the files in the chat's messages. */
export const ChatAttachments: React.FC<ChatAttachmentsProps> = ({
	attachments,
	evictedFileIds,
	hasEarlierAttachmentsError,
	isRetrying,
	onRetry,
}) => {
	const isComplete = attachments !== undefined && !hasEarlierAttachmentsError;
	return (
		<Collapsible defaultOpen className="flex flex-col gap-3">
			<SectionHeader
				level="section"
				label={
					<CollapsibleTrigger className="group inline-flex appearance-none items-center gap-1 rounded-md focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link">
						{isComplete
							? `Attachments (${attachments.length.toLocaleString("en-US")})`
							: "Attachments"}
						<ChevronDownIcon
							aria-hidden
							className="size-3.5 text-content-secondary"
						/>
					</CollapsibleTrigger>
				}
			/>
			{/* Stays mounted while loading so the open trigger's aria-controls resolves. */}
			<CollapsibleContent className="flex flex-col gap-3">
				{attachments && attachments.length > 0 && (
					<ul className="flex flex-col gap-2">
						{attachments.map(({ key, file, href }) => (
							<li key={key}>
								<FileCard
									block={file}
									href={href}
									expired={
										file.file_id !== undefined &&
										evictedFileIds.has(file.file_id)
									}
									fullWidth
								/>
							</li>
						))}
					</ul>
				)}
				{attachments === undefined && !hasEarlierAttachmentsError && (
					<Spinner loading size="sm" label="Loading attachments" />
				)}
				{isComplete && attachments.length === 0 && (
					<p className="m-0 text-sm text-content-secondary">
						No attachments yet.
					</p>
				)}
				{hasEarlierAttachmentsError && (
					<div className="flex flex-col items-start gap-2">
						<p className="m-0 text-sm text-content-secondary">
							Could not load attachments from earlier messages.
						</p>
						<RetryButton
							variant="outline"
							size="sm"
							isRetrying={isRetrying}
							onRetry={onRetry}
						/>
					</div>
				)}
			</CollapsibleContent>
		</Collapsible>
	);
};
