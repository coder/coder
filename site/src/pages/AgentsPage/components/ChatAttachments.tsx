import { FileIcon } from "lucide-react";
import { useId } from "react";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	expiredAttachmentExplanation,
	FileCard,
	getAttachmentDisplayName,
} from "./ChatConversation/AttachmentBlocks";
import type { MessageAttachment } from "./ChatConversation/messageHelpers";
import { RetryButton } from "./RetryButton";
import { SectionHeader } from "./SectionHeader";

type ChatAttachmentsProps = {
	/** Newest first; undefined until the first message page loads. */
	attachments: readonly MessageAttachment[] | undefined;
	evictedFileIds: ReadonlySet<string>;
	isLoadingEarlierAttachments: boolean;
	hasEarlierAttachmentsError: boolean;
	isRetrying: boolean;
	onRetry: () => void;
};

/**
 * Groups `attachments` by who attached them, in the order given, and folds
 * expired ones into a closed list. The header counts the files that still
 * exist once the list is complete.
 */
export const ChatAttachments: React.FC<ChatAttachmentsProps> = ({
	attachments,
	evictedFileIds,
	isLoadingEarlierAttachments,
	hasEarlierAttachmentsError,
	isRetrying,
	onRetry,
}) => {
	const isLoading = attachments === undefined || isLoadingEarlierAttachments;
	const isComplete = !isLoading && !hasEarlierAttachmentsError;
	const listed = attachments ?? [];
	const isExpired = ({ file }: MessageAttachment) =>
		file.file_id !== undefined && evictedFileIds.has(file.file_id);
	const available = listed.filter((attachment) => !isExpired(attachment));
	const expired = listed.filter(isExpired);
	return (
		<Collapsible defaultOpen className="flex flex-col gap-3">
			<SectionHeader
				level="section"
				label={
					<CollapsibleTrigger className="group inline-flex appearance-none items-center gap-1 rounded-md focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link">
						{isComplete
							? `Attachments (${available.length.toLocaleString("en-US")})`
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
				<AttachmentGroup
					label="From the agent"
					attachments={available.filter(({ role }) => role !== "user")}
				/>
				<AttachmentGroup
					label="From you"
					attachments={available.filter(({ role }) => role === "user")}
				/>
				{expired.length > 0 && <ExpiredAttachments attachments={expired} />}
				{isLoading && <Spinner loading size="sm" label="Loading attachments" />}
				{isComplete && listed.length === 0 && (
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
				{attachments.map(({ key, file, href }) => (
					<li key={key}>
						<FileCard block={file} href={href} expired={false} fullWidth />
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
			{`${attachments.length.toLocaleString("en-US")} expired`}
			<ChevronDownIcon aria-hidden className="size-3.5" />
		</CollapsibleTrigger>
		<CollapsibleContent className="flex flex-col gap-2">
			<p className="m-0 text-xs text-content-secondary">
				{expiredAttachmentExplanation}
			</p>
			<ul className="flex flex-col gap-1">
				{attachments.map(({ key, file }) => (
					<li
						key={key}
						className="flex min-w-0 items-center gap-2 text-sm text-content-secondary"
					>
						<FileIcon aria-hidden className="size-4 shrink-0" />
						<span className="truncate">{getAttachmentDisplayName(file)}</span>
					</li>
				))}
			</ul>
		</CollapsibleContent>
	</Collapsible>
);
