import type { FC } from "react";
import type { UrlTransform } from "streamdown";
import { cn } from "#/utils/cn";
import { Message, MessageContent } from "../ChatElements";
import {
	AttachmentBlock,
	type PreviewTextAttachment,
} from "./AttachmentBlocks";
import type { MessageDisplayState } from "./messageHelpers";
import { UserMessageMarkdown } from "./UserMessageMarkdown";

export const UserMessageContent: FC<{
	displayState: MessageDisplayState;
	markdown: string;
	urlTransform?: UrlTransform;
	isEditing?: boolean;
	onImageClick?: (src: string) => void;
	onTextFileClick?: (attachment: PreviewTextAttachment) => void;
}> = ({
	displayState,
	markdown,
	urlTransform,
	isEditing = false,
	onImageClick,
	onTextFileClick,
}) => {
	return (
		<Message className="w-fit max-w-[min(80vw,80%)]">
			<MessageContent
				className={cn(
					"rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans shadow-sm transition-shadow",
					isEditing &&
						"border-surface-secondary shadow-[0_0_0_2px_hsla(var(--border-warning),0.6)]",
				)}
			>
				<div className="flex flex-col gap-1.5">
					{displayState.hasUserMessageBody && (
						<UserMessageMarkdown
							blocks={displayState.userInlineContent}
							markdown={markdown}
							urlTransform={urlTransform}
						/>
					)}
					{displayState.hasFileBlocks && (
						<div
							className={cn(
								displayState.hasUserMessageBody && "mt-2",
								"flex flex-wrap gap-2",
							)}
						>
							{displayState.userFileBlocks.map((block, index) => (
								<AttachmentBlock
									key={`user-file-${block.file_id ?? index}`}
									block={block}
									onImageClick={onImageClick}
									onTextFileClick={onTextFileClick}
									showTextStatus
								/>
							))}
						</div>
					)}
				</div>
			</MessageContent>
		</Message>
	);
};
