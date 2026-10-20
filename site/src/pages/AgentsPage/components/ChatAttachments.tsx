import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { Spinner } from "#/components/Spinner/Spinner";
import { FileCard } from "./ChatConversation/AttachmentBlocks";
import type { AgentAttachment } from "./ChatConversation/messageHelpers";
import type { EarlierMessagesProps } from "./ChatMessageScroller";
import { SectionHeader } from "./SectionHeader";

type ChatAttachmentsProps = {
	/** Undefined until the chat's first message page reaches the store. */
	attachments: readonly AgentAttachment[] | undefined;
	evictedFileIds: ReadonlySet<string>;
} & Pick<
	EarlierMessagesProps,
	| "hasMoreMessages"
	| "isFetchingMoreMessages"
	| "hasFetchMoreError"
	| "onFetchMoreMessages"
>;

/** Collapsible download cards for the agent's attachments in the loaded messages. */
export const ChatAttachments: React.FC<ChatAttachmentsProps> = ({
	attachments,
	evictedFileIds,
	hasMoreMessages,
	isFetchingMoreMessages,
	hasFetchMoreError,
	onFetchMoreMessages,
}) => {
	return (
		<Collapsible defaultOpen className="flex flex-col gap-3">
			<SectionHeader
				level="section"
				label={
					<CollapsibleTrigger className="group inline-flex appearance-none items-center gap-1 rounded-md focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link">
						{attachments
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
				{attachments?.length === 0 && (
					<p className="m-0 text-sm text-content-secondary">
						{hasMoreMessages
							? "No attachments in the loaded messages."
							: "No attachments yet."}
					</p>
				)}
				{attachments && hasMoreMessages && (
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
