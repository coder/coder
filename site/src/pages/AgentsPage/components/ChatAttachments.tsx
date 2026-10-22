import { FileIcon } from "lucide-react";
import { useId } from "react";
import { useQueries } from "react-query";
import { chatFileText } from "#/api/queries/chats";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	expiredAttachmentExplanation,
	getAttachmentDisplayName,
} from "./ChatConversation/AttachmentBlocks";
import {
	getStoredTextFileId,
	isExpiredFileError,
	MessageAttachmentCard,
} from "./ChatConversation/MessageAttachmentCard";
import type { MessageAttachment } from "./ChatConversation/messageHelpers";
import { RetryButton } from "./RetryButton";
import { SectionHeader } from "./SectionHeader";

type ChatAttachmentsProps = {
	attachments: readonly AgentAttachment[];
	evictedFileIds: ReadonlySet<string>;
	isLoadingEarlierAttachments: boolean;
	earlierAttachmentsError: unknown;
	isRetrying: boolean;
	onRetry: () => void;
};

/**
 * Once the list is complete, the header count leaves out evicted files and
 * text files a fetch found deleted, while that 404 stays cached.
 */
export const ChatAttachments: React.FC<ChatAttachmentsProps> = ({
	attachments,
	evictedFileIds,
	isLoadingEarlierAttachments,
	earlierAttachmentsError,
	isRetrying,
	onRetry,
}) => {
	const isLoading = attachments === undefined || isLoadingEarlierAttachments;
	const isComplete = !isLoading && !earlierAttachmentsError;
	const listed = attachments ?? [];
	const isEvicted = ({ file }: MessageAttachment) =>
		file.file_id !== undefined && evictedFileIds.has(file.file_id);
	const kept = listed.filter((attachment) => !isEvicted(attachment));
	const evicted = listed.filter(isEvicted);
	const deletedTextFileCount = useQueries({
		queries: kept.flatMap(({ file }) => {
			const fileId = getStoredTextFileId(file);
			return fileId ? [chatFileText(fileId)] : [];
		}),
		combine: (results) =>
			results.filter(({ error }) => isExpiredFileError(error)).length,
	});
	const unexpiredCount = kept.length - deletedTextFileCount;
	return (
		<Collapsible defaultOpen className="flex flex-col gap-3">
			<SectionHeader
				level="section"
				label={
					<CollapsibleTrigger className="group inline-flex appearance-none items-center gap-1 rounded-md focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link">
						{`Attachments (${attachments.length.toLocaleString("en-US")})`}
						<ChevronDownIcon
							aria-hidden
							className="size-3.5 text-content-secondary"
						/>
					</CollapsibleTrigger>
				}
			/>
			<CollapsibleContent className="flex flex-col gap-3">
				{attachments.length > 0 && (
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
				{attachments.length === 0 && (
					<p className="m-0 text-sm text-content-secondary">
						No attachments yet.
					</p>
				)}
				{hasMoreMessages && (
					<Button
						variant="outline"
						size="sm"
						className="self-start"
						disabled={isFetchingMoreMessages}
						onClick={() => void onFetchMoreMessages()}
					>
						<Spinner loading={isFetchingMoreMessages} />
						{hasFetchMoreError
							? "Retry loading earlier messages"
							: "Load earlier messages"}
					</Button>
				)}
			</CollapsibleContent>
		</Collapsible>
	);
};

type AttachmentGroupProps = {
	label: string;
	attachments: readonly MessageAttachment[];
};

const AttachmentGroup: React.FC<AttachmentGroupProps> = ({
	label,
	attachments,
}) => {
	const headingId = useId();
	if (attachments.length === 0) {
		return null;
	}
	return (
		<section aria-labelledby={headingId} className="flex flex-col gap-2">
			<h4
				id={headingId}
				className="m-0 text-xs font-medium text-content-secondary"
			>
				{label}
			</h4>
			<ul className="flex flex-col gap-2">
				{attachments.map((attachment) => (
					<li key={attachment.key}>
						<MessageAttachmentCard attachment={attachment} />
					</li>
				))}
			</ul>
		</section>
	);
};

type ExpiredAttachmentsProps = {
	attachments: readonly MessageAttachment[];
};

const ExpiredAttachments: React.FC<ExpiredAttachmentsProps> = ({
	attachments,
}) => (
	<Collapsible className="flex flex-col gap-2">
		<CollapsibleTrigger className="group inline-flex appearance-none items-center gap-1 self-start rounded-md text-xs font-medium text-content-secondary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link">
			{attachments.length.toLocaleString("en-US")} expired{" "}
			{attachments.length === 1 ? "attachment" : "attachments"}
			<ChevronDownIcon aria-hidden className="size-3.5" />
		</CollapsibleTrigger>
		<CollapsibleContent className="flex flex-col gap-2">
			<p className="m-0 text-sm text-content-secondary">
				{expiredAttachmentExplanation}
			</p>
			<ul className="flex flex-col gap-1">
				{attachments.map(({ key, file }) => (
					<li
						key={key}
						className="flex items-center gap-2 text-sm text-content-secondary"
					>
						<FileIcon aria-hidden className="size-4 shrink-0" />
						<span className="truncate">{getAttachmentDisplayName(file)}</span>
					</li>
				))}
			</ul>
		</CollapsibleContent>
	</Collapsible>
);
