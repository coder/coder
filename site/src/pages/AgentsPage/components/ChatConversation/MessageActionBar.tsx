import { cn } from "cn";
import { Button } from "#/components/Button/Button";
import { CopyButton } from "#/components/CopyButton/CopyButton";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import type { TurnAttachmentList } from "./messageHelpers";
import { TurnAttachments } from "./TurnAttachments";

type MessageActionBarProps = React.PropsWithChildren<{
	isUser: boolean;
	/** The message's Markdown, when it has text to copy. */
	copyText: string | undefined;
	turnAttachments: TurnAttachmentList | undefined;
	showsHoverActions: boolean;
}>;

/**
 * The row under a message. Copy and the attachments pill stay visible, with
 * Copy on the side the message aligns to; the buttons passed as children
 * appear on hover or focus.
 */
export const MessageActionBar: React.FC<MessageActionBarProps> = ({
	isUser,
	copyText,
	turnAttachments,
	showsHoverActions,
	children,
}) => {
	const copyButton = copyText !== undefined && (
		<CopyButton
			text={copyText}
			label="Copy message"
			className="size-6"
			tooltipSide="bottom"
		/>
	);

	return (
		<div
			className={cn(
				"mt-0.5 flex items-center gap-1",
				isUser && "w-full justify-end",
			)}
		>
			{!isUser && copyButton}
			{turnAttachments && <TurnAttachments attachments={turnAttachments} />}
			{showsHoverActions && (
				<div className="flex items-center gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover/msg:opacity-100">
					{children}
				</div>
			)}
			{isUser && copyButton}
		</div>
	);
};

/** An icon button for the hover actions, labeled by its tooltip text. */
export const MessageActionButton: React.FC<{
	label: string;
	icon: React.ReactNode;
	onClick: () => void;
	disabled?: boolean;
}> = ({ label, icon, onClick, disabled }) => (
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
				<span className="sr-only">{label}</span>
			</Button>
		</TooltipTrigger>
		<TooltipContent side="bottom">{label}</TooltipContent>
	</Tooltip>
);
