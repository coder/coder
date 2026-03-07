import { UserIcon } from "lucide-react";
import type { FC } from "react";
import type { FileTree } from "utils/filetree";
import { BuildApprovalCard } from "./BuildApprovalCard";
import { MemoizedChatMarkdown } from "./ChatMarkdown";
import { EditApprovalCard } from "./EditApprovalCard";
import { PublishApprovalCard } from "./PublishApprovalCard";
import { ReasoningBlock } from "./ReasoningBlock";
import { ToolCallCard } from "./ToolCallCard";
import type { PendingToolCall } from "./types";
import type { DisplayMessage } from "./useTemplateAgent";

interface ChatMessageProps {
	message: DisplayMessage;
	pendingApproval: PendingToolCall | null;
	onApprove: () => void;
	onReject: () => void;
	onNavigateToFile?: (path: string) => void;
	getFileTree: () => FileTree;
}

export const ChatMessage: FC<ChatMessageProps> = ({
	message,
	pendingApproval,
	onApprove,
	onReject,
	onNavigateToFile,
	getFileTree,
}) => {
	if (message.role === "user") {
		return (
			<div className="flex justify-end">
				<div className="max-w-[90%] rounded-lg bg-surface-invert-primary px-3 py-2 text-sm text-content-invert">
					<div className="mb-1 flex items-center gap-1 text-2xs text-content-invert-secondary">
						<UserIcon className="size-3" />
						<span>You</span>
					</div>
					<MemoizedChatMarkdown>{message.content}</MemoizedChatMarkdown>
				</div>
			</div>
		);
	}

	return (
		<div className="space-y-2">
			{message.reasoning.length > 0 && (
				<ReasoningBlock reasoning={message.reasoning} />
			)}

			{message.content.trim().length > 0 && (
				<MemoizedChatMarkdown className="prose-invert">
					{message.content}
				</MemoizedChatMarkdown>
			)}

			{message.toolCalls.map((toolCall) => {
				const isEditAction =
					toolCall.toolName === "editFile" ||
					toolCall.toolName === "deleteFile";
				const isBuildAction = toolCall.toolName === "buildTemplate";

				if (isEditAction) {
					const isPending =
						pendingApproval?.toolCallId === toolCall.toolCallId &&
						toolCall.state === "pending";
					return (
						<EditApprovalCard
							key={toolCall.toolCallId}
							toolCall={toolCall}
							isPending={isPending}
							onApprove={onApprove}
							onReject={onReject}
							onNavigateToFile={onNavigateToFile}
							getFileTree={getFileTree}
						/>
					);
				}

				if (isBuildAction) {
					const isPending =
						pendingApproval?.toolCallId === toolCall.toolCallId &&
						toolCall.state === "pending";
					return (
						<BuildApprovalCard
							key={toolCall.toolCallId}
							toolCall={toolCall}
							isPending={isPending}
							onApprove={onApprove}
							onReject={onReject}
						/>
					);
				}

				const isPublishAction = toolCall.toolName === "publishTemplate";

				if (isPublishAction) {
					const isPending =
						pendingApproval?.toolCallId === toolCall.toolCallId &&
						toolCall.state === "pending";
					return (
						<PublishApprovalCard
							key={toolCall.toolCallId}
							toolCall={toolCall}
							isPending={isPending}
							onApprove={onApprove}
							onReject={onReject}
						/>
					);
				}

				return (
					<ToolCallCard
						key={toolCall.toolCallId}
						toolCall={toolCall}
						onNavigateToFile={onNavigateToFile}
					/>
				);
			})}
		</div>
	);
};
