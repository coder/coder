import { cn } from "cn";
import type { TurnAttachmentList } from "./messageHelpers";
import { TurnAttachments } from "./TurnAttachments";

type MessageActionBarProps = React.PropsWithChildren<{
	isUser: boolean;
	turnAttachments: TurnAttachmentList | undefined;
	showsActions: boolean;
}>;

/**
 * The row under a message. The attachments pill stays visible; the action
 * buttons passed as children appear on hover or focus.
 */
export const MessageActionBar: React.FC<MessageActionBarProps> = ({
	isUser,
	turnAttachments,
	showsActions,
	children,
}) => (
	<div
		className={cn(
			"mt-0.5 flex items-center gap-1",
			isUser && "w-full justify-end",
		)}
	>
		{turnAttachments && <TurnAttachments attachments={turnAttachments} />}
		{showsActions && (
			<div
				className="flex items-center gap-0.5 opacity-0 transition-opacity focus-within:opacity-100 group-hover/msg:opacity-100"
				data-testid="message-actions"
			>
				{children}
			</div>
		)}
	</div>
);
