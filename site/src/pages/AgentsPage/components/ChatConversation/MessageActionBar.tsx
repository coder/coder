import { cn } from "cn";
import type { AgentAttachment } from "./messageHelpers";
import { TurnAttachments } from "./TurnAttachments";

type MessageActionBarProps = React.PropsWithChildren<{
	isUser: boolean;
	/** Set on the row that hosts a settled turn's attachments. */
	turnAttachments?: readonly AgentAttachment[];
	/** Whether to render the hover-revealed action buttons in children. */
	showsActions: boolean;
}>;

/** Row below a message: the turn's attachments pill, then its actions. */
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
