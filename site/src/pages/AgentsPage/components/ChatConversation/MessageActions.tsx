import { cn } from "cn";
import { ChevronLeftIcon, ChevronRightIcon, PencilIcon } from "lucide-react";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import type { MessageAttachment } from "./messageHelpers";
import { getEditableUserMessagePayload } from "./messageParsing";
import { TurnAttachments } from "./TurnAttachments";

type MessageActionBarProps = {
	className?: string;
	children: React.ReactNode;
};

/**
 * Message rows and working blocks share it, so a turn's pill sits at the same
 * offset under either.
 */
export const MessageActionBar: React.FC<MessageActionBarProps> = ({
	className,
	children,
}) => (
	<div className={cn("mt-0.5 flex items-center gap-1", className)}>
		{children}
	</div>
);

type MessageActionButtonProps = {
	label: string;
	icon: React.ReactNode;
	onClick: () => void;
	disabled?: boolean;
};

const MessageActionButton: React.FC<MessageActionButtonProps> = ({
	label,
	icon,
	onClick,
	disabled,
}) => (
	<Tooltip>
		<TooltipTrigger asChild>
			<Button
				size="icon"
				variant="subtle"
				className="size-6"
				aria-label={label}
				disabled={disabled}
				onClick={onClick}
			>
				{icon}
			</Button>
		</TooltipTrigger>
		<TooltipContent side="bottom">{label}</TooltipContent>
	</Tooltip>
);

type UserMessageActionsProps = {
	message: TypesGen.ChatMessage;
	copyText?: string;
	onEditUserMessage?: (
		messageId: number,
		text: string,
		fileBlocks?: readonly TypesGen.ChatMessagePart[],
		editorState?: string,
	) => void;
	prevUserMessageKey?: string;
	nextUserMessageKey?: string;
	onJumpToUserMessage?: (messageKey: string) => void;
};

/** Copy stays visible; Edit and jump show on hover or focus. */
export const UserMessageActions: React.FC<UserMessageActionsProps> = ({
	message,
	copyText,
	onEditUserMessage,
	prevUserMessageKey,
	nextUserMessageKey,
	onJumpToUserMessage,
}) => {
	const canJump =
		onJumpToUserMessage !== undefined &&
		(prevUserMessageKey !== undefined || nextUserMessageKey !== undefined);
	const hasHoverActions = onEditUserMessage !== undefined || canJump;
	if (!hasHoverActions && copyText === undefined) {
		return null;
	}

	return (
		<MessageActionBar className="w-full justify-end">
			{hasHoverActions && (
				<div className="flex items-center gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover/msg:opacity-100">
					{onEditUserMessage && (
						<MessageActionButton
							label="Edit message"
							icon={<PencilIcon />}
							onClick={() => {
								const { text, fileBlocks, editorState } =
									getEditableUserMessagePayload(message);
								onEditUserMessage(message.id, text, fileBlocks, editorState);
							}}
						/>
					)}
					{canJump && (
						<>
							<MessageActionButton
								label="Jump to previous user message"
								icon={<ChevronLeftIcon />}
								disabled={prevUserMessageKey === undefined}
								onClick={() => {
									if (prevUserMessageKey !== undefined) {
										onJumpToUserMessage(prevUserMessageKey);
									}
								}}
							/>
							<MessageActionButton
								label="Jump to next user message"
								icon={<ChevronRightIcon />}
								disabled={nextUserMessageKey === undefined}
								onClick={() => {
									if (nextUserMessageKey !== undefined) {
										onJumpToUserMessage(nextUserMessageKey);
									}
								}}
							/>
						</>
					)}
				</div>
			)}
			{copyText !== undefined && (
				<CopyButton
					text={copyText}
					label="Copy message"
					className="size-6"
					tooltipSide="bottom"
				/>
			)}
		</MessageActionBar>
	);
};

type AssistantMessageActionsProps = {
	copyText?: string;
	turnAttachments?: readonly MessageAttachment[];
	keepsSpacing: boolean;
};

/**
 * Without Copy or the pill, keepsSpacing renders a spacer of the bar's height,
 * so the gap to the next message stays even.
 */
export const AssistantMessageActions: React.FC<
	AssistantMessageActionsProps
> = ({ copyText, turnAttachments, keepsSpacing }) => {
	if (copyText === undefined && turnAttachments === undefined) {
		return keepsSpacing ? (
			<div className="min-h-6" data-testid="assistant-bottom-spacer" />
		) : null;
	}

	return (
		<MessageActionBar>
			{copyText !== undefined && (
				<CopyButton
					text={copyText}
					label="Copy message"
					className="size-6"
					tooltipSide="bottom"
				/>
			)}
			{turnAttachments && <TurnAttachments attachments={turnAttachments} />}
		</MessageActionBar>
	);
};
