import { cn } from "cn";
import {
	ArchiveIcon,
	ArchiveRestoreIcon,
	BotIcon,
	ExternalLinkIcon,
	GitPullRequestIcon,
	PinIcon,
	PinOffIcon,
	SquarePenIcon,
	Trash2Icon,
} from "lucide-react";
import { type FC, useId } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import type {
	ContextMenuItem,
	ContextMenuSeparator,
	ContextMenuSub,
	ContextMenuSubContent,
	ContextMenuSubTrigger,
} from "#/components/ContextMenu/ContextMenu";
import type {
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { limitPRMenuHeight } from "../utils/limitPRMenuHeight";
import { originRepoLabel } from "../utils/originRepoLabel";
import { parsePullRequestUrl } from "../utils/pullRequest";
import { getParentChatID } from "./ChatConversation/chatHelpers";
import { getPRIconConfig } from "./ChatsSidebar/tree/statusConfig";

// Backend chatstate permits archive only from W, E0, and E1. Unknown status
// stays fail-open so the server conflict response remains the backstop.
const chatStatusAllowsArchive = (
	status: TypesGen.ChatStatus | null | undefined,
): boolean =>
	status === undefined ||
	status === null ||
	status === "waiting" ||
	status === "error";

// Archive cascades atomically over the whole family, so the backend
// rejects it when any child is still active, not just the root. Children
// are embedded on chat records (depth capped at 1); a missing children
// array stays fail-open like an unknown status.
export const chatFamilyAllowsArchive = (
	status: TypesGen.ChatStatus | null | undefined,
	children: readonly TypesGen.Chat[] | null | undefined,
): boolean =>
	chatStatusAllowsArchive(status) &&
	(children ?? []).every((child) => chatStatusAllowsArchive(child.status));

type ItemComponent = typeof DropdownMenuItem | typeof ContextMenuItem;
type SeparatorComponent =
	| typeof DropdownMenuSeparator
	| typeof ContextMenuSeparator;

/** The submenu primitives that render the flyout for several PRs. */
export type PullRequestSubmenuComponents = {
	readonly Sub: typeof DropdownMenuSub | typeof ContextMenuSub;
	readonly SubTrigger:
		| typeof DropdownMenuSubTrigger
		| typeof ContextMenuSubTrigger;
	readonly SubContent:
		| typeof DropdownMenuSubContent
		| typeof ContextMenuSubContent;
	/** Extra classes for the flyout, such as the mobile placement. */
	readonly contentClassName?: string;
};

/**
 * Pin, rename, and archive write to the chat record itself, so from a shared
 * chat they would change the owner's sidebar and title. Sharing only grants
 * read access, and admins who would pass the server's update check should
 * not manage another user's chat from a shared view either, so ownership
 * rather than authorization decides who sees those actions.
 */
export const canManageChat = (
	chat: TypesGen.Chat,
	currentUserId: string,
): boolean => chat.owner_id === currentUserId;

type ChatMenuActionsOptions = {
	readonly canManage: boolean;
	/** Whether the menu offers the subagents toggle, a viewer action. */
	readonly hasSubagentsToggle?: boolean;
	/** Whether the menu links to pull requests, a viewer action. */
	readonly hasPullRequests?: boolean;
};

/**
 * Archive state is root-only on the backend and cascades to children, so
 * child chats expose no archive or unarchive actions. An archived child chat
 * therefore has no menu actions at all, and a non-owner only has the
 * subagents toggle and pull request links; call sites use this to hide the
 * menu trigger instead of rendering an empty menu.
 */
export const chatHasMenuActions = (
	chat: TypesGen.Chat,
	{
		canManage,
		hasSubagentsToggle = false,
		hasPullRequests = false,
	}: ChatMenuActionsOptions,
): boolean => {
	if (!canManage) {
		return hasSubagentsToggle || hasPullRequests;
	}
	const isArchivedChild = chat.archived && getParentChatID(chat) !== undefined;
	return !isArchivedChild;
};

type ChatActionsMenuItemsProps = {
	readonly chat: TypesGen.Chat;
	/**
	 * See {@link canManageChat}. When false, only the subagents toggle and
	 * pull request links render.
	 */
	readonly canManage: boolean;
	readonly hasWorkspace: boolean;
	readonly isArchiving?: boolean;
	readonly isArchiveBlocked?: boolean;
	readonly subagentCount?: number;
	readonly isSubagentsExpanded?: boolean;
	readonly onToggleSubagents?: () => void;
	readonly onPinAgent?: () => void;
	readonly onUnpinAgent?: () => void;
	readonly onArchiveAgent: () => void;
	readonly onUnarchiveAgent: () => void;
	readonly onArchiveAndDeleteWorkspace: () => void;
	/** When omitted, the "Rename chat" item is hidden. */
	readonly onOpenRenameDialog?: () => void;
	/**
	 * Pull requests to link to. One renders inline; several render in a
	 * flyout, which needs `submenu`.
	 */
	readonly pullRequests?: readonly TypesGen.ChatDiffStatus[];
	readonly submenu?: PullRequestSubmenuComponents;
	readonly Item: ItemComponent;
	readonly Separator: SeparatorComponent;
};

export const ChatActionsMenuItems: FC<ChatActionsMenuItemsProps> = ({
	chat,
	canManage,
	hasWorkspace,
	isArchiving = false,
	isArchiveBlocked = false,
	subagentCount = 0,
	isSubagentsExpanded = false,
	onToggleSubagents,
	onPinAgent,
	onUnpinAgent,
	onArchiveAgent,
	onUnarchiveAgent,
	onArchiveAndDeleteWorkspace,
	onOpenRenameDialog,
	pullRequests = [],
	submenu,
	Item,
	Separator,
}) => {
	const isArchived = chat.archived;
	const isPinned = chat.pin_order > 0;
	const isChildChat = getParentChatID(chat) !== undefined;
	const showSubagentsToggle = Boolean(onToggleSubagents) && subagentCount > 0;
	const showPinAction =
		!isArchived && !isChildChat && Boolean(onPinAgent && onUnpinAgent);
	const showArchiveActions = !isArchived && !isChildChat;
	const archiveBlockedHintId = useId();
	const archiveBlockedDescribedBy = isArchiveBlocked
		? archiveBlockedHintId
		: undefined;

	const subagentToggle = showSubagentsToggle ? (
		<Item onSelect={onToggleSubagents}>
			<BotIcon className="size-3.5" />
			{isSubagentsExpanded
				? "Hide subagents"
				: `Show subagents (${subagentCount})`}
		</Item>
	) : null;

	const hasPullRequestItems =
		pullRequests.length === 1 ||
		(pullRequests.length > 1 && submenu !== undefined);
	const pullRequestItems = hasPullRequestItems ? (
		<PullRequestMenuItems
			pullRequests={pullRequests}
			submenu={submenu}
			Item={Item}
		/>
	) : null;

	if (!canManage) {
		return (
			<>
				{subagentToggle}
				{subagentToggle && pullRequestItems && <Separator />}
				{pullRequestItems}
			</>
		);
	}

	const hasLeadingItems =
		onOpenRenameDialog || showPinAction || showSubagentsToggle;

	return (
		<>
			{showPinAction && (
				<Item onSelect={isPinned ? onUnpinAgent : onPinAgent}>
					{isPinned ? (
						<>
							<PinOffIcon className="size-3.5" />
							Unpin agent
						</>
					) : (
						<>
							<PinIcon className="size-3.5" />
							Pin agent
						</>
					)}
				</Item>
			)}
			{isArchived ? (
				!isChildChat && (
					<>
						<Item disabled={isArchiving} onSelect={onUnarchiveAgent}>
							<ArchiveRestoreIcon className="size-3.5" />
							Unarchive agent
						</Item>
						{subagentToggle}
						{pullRequestItems && <Separator />}
						{pullRequestItems}
					</>
				)
			) : (
				<>
					{onOpenRenameDialog && (
						<Item onSelect={onOpenRenameDialog}>
							<SquarePenIcon className="size-3.5" />
							Rename chat
						</Item>
					)}
					{subagentToggle}
					{hasLeadingItems && pullRequestItems && <Separator />}
					{pullRequestItems}
					{showArchiveActions && (
						<>
							{(hasLeadingItems || pullRequestItems) && <Separator />}
							<Item
								className="text-content-destructive focus:text-content-destructive"
								aria-describedby={archiveBlockedDescribedBy}
								disabled={isArchiving || isArchiveBlocked}
								onSelect={onArchiveAgent}
							>
								<ArchiveIcon className="size-3.5" />
								Archive agent
							</Item>
							{hasWorkspace && (
								<Item
									className="text-content-destructive focus:text-content-destructive"
									aria-describedby={archiveBlockedDescribedBy}
									disabled={isArchiving || isArchiveBlocked}
									onSelect={onArchiveAndDeleteWorkspace}
								>
									<Trash2Icon className="size-3.5" />
									Archive & delete workspace
								</Item>
							)}
							{isArchiveBlocked && (
								<div
									id={archiveBlockedHintId}
									className="max-w-56 px-2 py-1.5 text-xs text-content-secondary"
								>
									Interrupt or wait for the agent to finish first.
								</div>
							)}
						</>
					)}
				</>
			)}
		</>
	);
};

type PullRequestMenuItemsProps = {
	readonly pullRequests: readonly TypesGen.ChatDiffStatus[];
	readonly submenu: PullRequestSubmenuComponents | undefined;
	readonly Item: ItemComponent;
};

/** Classes for a menu that lists PRs; pair with `limitPRMenuHeight`. */
export const pullRequestMenuContentClassName =
	"relative w-72 overflow-y-auto [&_[role=menuitem]]:text-[13px]";

const PullRequestMenuItems: FC<PullRequestMenuItemsProps> = ({
	pullRequests,
	submenu,
	Item,
}) => {
	const items = <PullRequestMenuList pullRequests={pullRequests} Item={Item} />;

	if (pullRequests.length === 1) {
		return items;
	}
	if (!submenu) {
		return null;
	}
	const { Sub, SubTrigger, SubContent, contentClassName } = submenu;
	return (
		<Sub>
			<SubTrigger>
				<GitPullRequestIcon className="size-3.5" />
				{pullRequests.length} PRs
			</SubTrigger>
			<SubContent
				ref={limitPRMenuHeight}
				className={cn(pullRequestMenuContentClassName, contentClassName)}
			>
				{items}
			</SubContent>
		</Sub>
	);
};

type PullRequestMenuListProps = {
	readonly pullRequests: readonly TypesGen.ChatDiffStatus[];
	readonly Item: ItemComponent;
};

/** One single-line link row per PR. */
export const PullRequestMenuList: FC<PullRequestMenuListProps> = ({
	pullRequests,
	Item,
}) => {
	// The visible label omits the repository, so screen readers get it
	// when PR numbers could collide across repositories.
	const showRepository =
		new Set(pullRequests.map((pr) => pr.remote_origin).filter(Boolean)).size >
		1;
	return pullRequests.map((pr) => (
		<PullRequestMenuItem
			key={`${pr.remote_origin}/${pr.git_branch}`}
			pullRequest={pr}
			showRepository={showRepository}
			Item={Item}
		/>
	));
};

type PullRequestMenuItemProps = {
	readonly pullRequest: TypesGen.ChatDiffStatus;
	readonly showRepository: boolean;
	readonly Item: ItemComponent;
};

const PullRequestMenuItem: FC<PullRequestMenuItemProps> = ({
	pullRequest,
	showRepository,
	Item,
}) => {
	const config = getPRIconConfig(pullRequest);
	const Icon = config?.icon ?? GitPullRequestIcon;
	const number =
		pullRequest.pr_number ?? parsePullRequestUrl(pullRequest.url)?.number;
	const title = pullRequest.pull_request_title.trim();
	const repository = showRepository
		? originRepoLabel(pullRequest.remote_origin)
		: "";

	return (
		<Item asChild>
			<a
				href={pullRequest.url}
				target="_blank"
				rel="noreferrer"
				className="min-w-0 no-underline"
			>
				<Icon
					aria-hidden="true"
					className={cn("size-3.5 shrink-0", config?.className)}
				/>
				{config && <span className="sr-only">{config.label}:</span>}
				<span className="shrink-0 text-content-primary">
					{number ? `PR #${number}` : "Pull request"}
				</span>
				{title && (
					<span className="min-w-0 flex-1 truncate text-content-secondary">
						{title}
					</span>
				)}
				{repository && <span className="sr-only">in {repository}</span>}
				<ExternalLinkIcon
					aria-hidden="true"
					className="ml-auto size-3.5! shrink-0 text-content-secondary"
				/>
			</a>
		</Item>
	);
};
