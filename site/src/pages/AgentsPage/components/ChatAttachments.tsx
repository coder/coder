import { ChevronDownIcon } from "lucide-react";
import { Button } from "#/components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { Spinner } from "#/components/Spinner/Spinner";
import { FileCard } from "./ChatConversation/AttachmentBlocks";
import { FileProbeProvider } from "./ChatConversation/FileProbeContext";
import type { AgentAttachment } from "./ChatConversation/messageHelpers";
import type { EarlierMessagesProps } from "./ChatMessageScroller";

type ChatAttachmentsProps = {
	attachments: readonly AgentAttachment[];
	evictedFileIds: ReadonlySet<string>;
} & Pick<
	EarlierMessagesProps,
	| "hasMoreMessages"
	| "isFetchingMoreMessages"
	| "isHydratingMessages"
	| "hasFetchMoreError"
	| "onFetchMoreMessages"
>;

/** Collapsible download cards for the agent's attachments in the loaded messages. */
export const ChatAttachments: React.FC<ChatAttachmentsProps> = ({
	attachments,
	evictedFileIds,
	hasMoreMessages,
	isFetchingMoreMessages,
	isHydratingMessages,
	hasFetchMoreError,
	onFetchMoreMessages,
}) => {
	return (
		<Collapsible defaultOpen className="flex flex-col gap-3">
			<h3 className="m-0">
				<CollapsibleTrigger asChild>
					<button
						type="button"
						className="group flex w-full cursor-pointer appearance-none items-center justify-between gap-2 rounded-md border-0 bg-transparent p-0 text-left font-sans text-sm font-medium text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
					>
						{isHydratingMessages
							? "Attachments"
							: `Attachments (${attachments.length.toLocaleString("en-US")})`}
						<ChevronDownIcon
							aria-hidden="true"
							className="size-3.5 shrink-0 text-content-secondary transition-transform group-data-[state=open]:rotate-180"
						/>
					</button>
				</CollapsibleTrigger>
			</h3>
			<CollapsibleContent className="flex flex-col gap-3">
				{attachments.length > 0 ? (
					<FileProbeProvider evictedFileIds={evictedFileIds}>
						<ul className="flex flex-col gap-2">
							{attachments.map(({ key, file, href }) => (
								<li key={key}>
									<FileCard block={file} href={href} fullWidth />
								</li>
							))}
						</ul>
					</FileProbeProvider>
				) : (
					!isHydratingMessages && (
						<p className="m-0 text-sm text-content-secondary">
							{hasMoreMessages
								? "No attachments in the loaded messages."
								: "No attachments yet."}
						</p>
					)
				)}
				{hasMoreMessages && (
					<Button
						variant="outline"
						size="sm"
						className="self-start"
						disabled={isFetchingMoreMessages || isHydratingMessages}
						onClick={() => void onFetchMoreMessages()}
					>
						<Spinner loading={isFetchingMoreMessages} aria-hidden />
						{hasFetchMoreError
							? "Retry loading earlier messages"
							: "Load earlier messages"}
					</Button>
				)}
			</CollapsibleContent>
		</Collapsible>
	);
};
