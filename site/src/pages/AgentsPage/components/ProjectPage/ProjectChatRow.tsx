import { cn } from "cn";
import { EllipsisVerticalIcon } from "lucide-react";
import { Link } from "react-router";
import type { Chat } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import type { AgentsPageOutletContext } from "../../AgentsPageLayout";
import { buildAgentChatPath } from "../../utils/navigation";
import {
	ChatActionsMenuItems,
	canManageChat,
	chatFamilyAllowsArchive,
	chatHasMenuActions,
} from "../ChatActionsMenuItems";
import { ChatDiffStats } from "../ChatDiffStats";
import { getChatDisplayConfig } from "../ChatsSidebar/tree/statusConfig";

/** The chat actions a project chat row offers from its menu. */
export type ProjectChatRowActions = Pick<
	AgentsPageOutletContext,
	| "requestArchiveAgent"
	| "requestUnarchiveAgent"
	| "requestArchiveAndDeleteWorkspace"
	| "requestPinAgent"
	| "requestUnpinAgent"
	| "onOpenRenameDialog"
	| "isArchiving"
>;

type ProjectChatRowProps = {
	readonly chat: Chat;
	readonly currentUserId: string;
	/** Omit to hide the actions menu. */
	readonly actions?: ProjectChatRowActions;
};

export const ProjectChatRow: React.FC<ProjectChatRowProps> = ({
	chat,
	currentUserId,
	actions,
}) => {
	const {
		icon: StatusIcon,
		className: statusClassName,
		label: statusLabel,
	} = getChatDisplayConfig(chat);
	const canManage = canManageChat(chat, currentUserId);
	const ownerLabel =
		chat.owner_id === currentUserId
			? "you"
			: chat.owner_name || chat.owner_username || "Unknown";
	const workspaceId = chat.workspace_id;

	return (
		<li className="flex min-w-0 items-center gap-2 pr-2 hover:bg-surface-secondary has-[[data-state=open]]:bg-surface-secondary">
			<Link
				to={buildAgentChatPath({ chatId: chat.id })}
				className="flex min-w-0 flex-1 items-center gap-3 py-2.5 pl-4 text-sm text-content-primary no-underline"
			>
				<StatusIcon
					role="img"
					aria-label={statusLabel}
					className={cn("size-3.5 shrink-0", statusClassName)}
				/>
				<span className="min-w-0 flex-1 truncate">{chat.title}</span>
				<span className="flex shrink-0 items-center gap-1.5">
					<ChatDiffStats chat={chat} />
				</span>
				<span className="max-w-40 shrink-0 truncate text-xs text-content-secondary">
					{ownerLabel}
				</span>
			</Link>
			{actions && !chatHasMenuActions(chat, { canManage }) && (
				// Keeps the owner column aligned with rows that have a menu.
				<span aria-hidden="true" className="size-7 shrink-0" />
			)}
			{actions && chatHasMenuActions(chat, { canManage }) && (
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button
							variant="subtle"
							size="icon"
							className="size-7 shrink-0"
							aria-label={`Open chat actions for ${chat.title}`}
						>
							<EllipsisVerticalIcon />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						<ChatActionsMenuItems
							Item={DropdownMenuItem}
							Separator={DropdownMenuSeparator}
							chat={chat}
							canManage={canManage}
							hasWorkspace={Boolean(workspaceId)}
							isArchiving={actions.isArchiving}
							isArchiveBlocked={
								!chatFamilyAllowsArchive(chat.status, chat.children)
							}
							onPinAgent={() => actions.requestPinAgent(chat.id)}
							onUnpinAgent={() => actions.requestUnpinAgent(chat.id)}
							onArchiveAgent={() => actions.requestArchiveAgent(chat.id)}
							onUnarchiveAgent={() => actions.requestUnarchiveAgent(chat.id)}
							onArchiveAndDeleteWorkspace={() => {
								if (workspaceId) {
									actions.requestArchiveAndDeleteWorkspace(
										chat.id,
										workspaceId,
									);
								}
							}}
							onOpenRenameDialog={
								actions.onOpenRenameDialog
									? () => actions.onOpenRenameDialog?.(chat)
									: undefined
							}
						/>
					</DropdownMenuContent>
				</DropdownMenu>
			)}
		</li>
	);
};
