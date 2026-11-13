import {
	ArchiveIcon,
	CircleAlertIcon,
	EllipsisVerticalIcon,
	LockIcon,
	PinIcon,
	PinOffIcon,
	UsersIcon,
} from "lucide-react";
import { useId, useRef } from "react";
import { Link, type To } from "react-router";
import type { ChatProject, ChatProjectSummary } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { shortRelativeTime } from "#/utils/time";
import { ChatProjectIcon } from "../components/ChatProjectIcon";
import {
	hasProjectManageAction,
	ProjectActionsMenuItems,
} from "../components/ChatsSidebar/chats/ProjectActionsMenuItems";
import { getChatStatusDisplay } from "../components/ChatsSidebar/tree/statusConfig";
import { useUnarchiveChatProject } from "../hooks/useUnarchiveChatProject";

type DialogAction = (returnFocusTo: HTMLElement | null) => void;

type ProjectCardProps = {
	readonly summary: ChatProjectSummary;
	readonly to: To;
	readonly onPin: () => void;
	readonly onUnpin: () => void;
	/** A pin request for the project is running, so Pin is held back. */
	readonly pinPending: boolean;
	readonly onEdit: DialogAction;
	readonly onArchive: DialogAction;
	readonly onDelete: DialogAction;
};

const compactNumber = new Intl.NumberFormat("en-US", {
	notation: "compact",
	maximumFractionDigits: 1,
});
const runningStatus = getChatStatusDisplay("running");

/**
 * A project on the Projects page. The whole card links to the project, and
 * its pin and actions buttons sit above that link.
 */
export const ProjectCard: React.FC<ProjectCardProps> = ({
	summary,
	to,
	onPin,
	onUnpin,
	pinPending,
	onEdit,
	onArchive,
	onDelete,
}) => {
	const { project, pinned, member_count, last_activity_at } = summary;
	return (
		<ProjectCardFrame
			project={project}
			to={to}
			footer={
				<>
					<ActivityLabel summary={summary} />
					{member_count > 1 ? (
						<span className="flex shrink-0 items-center gap-1">
							<UsersIcon aria-hidden="true" className="size-3.5" />
							<span aria-hidden="true" className="tabular-nums">
								{compactNumber.format(member_count).toLowerCase()}
							</span>
							<span className="sr-only">
								{member_count.toLocaleString("en-US")} members
							</span>
						</span>
					) : (
						<span className="flex shrink-0 items-center">
							<LockIcon aria-hidden="true" className="size-3.5" />
							<span className="sr-only">Only you</span>
						</span>
					)}
					{last_activity_at && <CardTime time={last_activity_at} />}
				</>
			}
			actions={
				<ProjectCardActions
					project={project}
					pin={{ pinned, pinPending, onPin, onUnpin }}
					onEdit={onEdit}
					onArchive={onArchive}
					onDelete={onDelete}
				/>
			}
		/>
	);
};

type ArchivedProjectCardProps = {
	readonly project: ChatProject;
	readonly to: To;
	/** Runs once the project is unarchived and has left the archived list. */
	readonly onUnarchived: () => void;
	readonly onDelete: DialogAction;
};

/**
 * An archived project on the Projects page. It links to the read-only
 * project page, and its actions unarchive or delete the project.
 */
export const ArchivedProjectCard: React.FC<ArchivedProjectCardProps> = ({
	project,
	to,
	onUnarchived,
	onDelete,
}) => {
	const { unarchive } = useUnarchiveChatProject(project, onUnarchived);
	return (
		<ProjectCardFrame
			project={project}
			to={to}
			footer={
				<>
					<span className="flex shrink-0 items-center gap-1">
						<ArchiveIcon aria-hidden="true" className="size-3.5" />
						Archived
					</span>
					{project.archived_at && <CardTime time={project.archived_at} />}
				</>
			}
			actions={
				hasProjectManageAction(project) && (
					<ProjectCardActions
						project={project}
						onUnarchive={unarchive}
						onDelete={onDelete}
					/>
				)
			}
		/>
	);
};

type ProjectCardFrameProps = {
	readonly project: ChatProject;
	readonly to: To;
	/** The footer row, which also describes the card's link. */
	readonly footer: React.ReactNode;
	readonly actions: React.ReactNode;
};

const ProjectCardFrame: React.FC<ProjectCardFrameProps> = ({
	project,
	to,
	footer,
	actions,
}) => {
	const footerId = useId();
	return (
		<div className="group relative flex min-w-0 flex-col gap-3 rounded-lg border border-solid border-border bg-surface-primary p-4 hover:bg-surface-secondary has-[a:focus-visible]:ring-2 has-[a:focus-visible]:ring-content-link">
			<div className="flex size-8 shrink-0 items-center justify-center rounded-md border border-solid border-border bg-surface-secondary">
				<ChatProjectIcon project={project} className="size-4" />
			</div>
			<div className="flex flex-col gap-1">
				<h2 className="m-0 truncate text-sm font-semibold">
					<Link
						to={to}
						aria-describedby={footerId}
						// Shows the full name when it is truncated.
						title={project.name}
						// The overlay makes the whole card the link's hit area.
						className="text-content-primary no-underline outline-hidden after:absolute after:inset-0 after:rounded-lg"
					>
						{project.name}
					</Link>
				</h2>
				{project.description && (
					<p className="m-0 line-clamp-2 text-sm text-content-secondary wrap-anywhere">
						{project.description}
					</p>
				)}
			</div>
			<div
				id={footerId}
				className="mt-auto flex items-center gap-3 text-xs text-content-secondary"
			>
				{footer}
			</div>
			{/* Follows the link in tab order. */}
			{actions}
		</div>
	);
};

type CardTimeProps = {
	readonly time: string;
};

const CardTime: React.FC<CardTimeProps> = ({ time }) => (
	<time
		dateTime={time}
		data-pixel="ignore"
		className="ml-auto shrink-0 tabular-nums"
	>
		{shortRelativeTime(time)}
	</time>
);

type ProjectCardActionsProps = {
	readonly project: ChatProject;
	/** The pin button and pin items, which archived projects lack. */
	readonly pin?: {
		readonly pinned: boolean;
		readonly pinPending: boolean;
		readonly onPin: () => void;
		readonly onUnpin: () => void;
	};
	readonly onEdit?: DialogAction;
	readonly onArchive?: DialogAction;
	readonly onUnarchive?: () => void;
	readonly onDelete: DialogAction;
};

const ProjectCardActions: React.FC<ProjectCardActionsProps> = ({
	project,
	pin,
	onEdit,
	onArchive,
	onUnarchive,
	onDelete,
}) => {
	const actionsButtonRef = useRef<HTMLButtonElement>(null);
	// Edit, Archive, and Delete open a dialog only once the menu has closed,
	// so the closing menu cannot take focus back from the dialog.
	const openPickedDialogRef = useRef<DialogAction>(undefined);
	const pickDialog = (openDialog: DialogAction) => () => {
		openPickedDialogRef.current = openDialog;
	};
	return (
		<div className="absolute top-4 right-4 z-10 flex h-8 items-center">
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button
						ref={actionsButtonRef}
						variant="subtle"
						size="icon"
						aria-label={`Open project actions for ${project.name}`}
						className="size-7 max-w-0 overflow-hidden px-0 [@media(hover:none)]:max-w-7 group-hover:max-w-7 group-focus-within:max-w-7 data-[state=open]:max-w-7"
					>
						<EllipsisVerticalIcon />
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent
					align="end"
					onCloseAutoFocus={(event) => {
						const openDialog = openPickedDialogRef.current;
						openPickedDialogRef.current = undefined;
						if (openDialog) {
							event.preventDefault();
							openDialog(actionsButtonRef.current);
						}
					}}
				>
					<ProjectActionsMenuItems
						Item={DropdownMenuItem}
						Separator={DropdownMenuSeparator}
						onPin={
							pin && !pin.pinned && !pin.pinPending ? pin.onPin : undefined
						}
						onUnpin={pin?.pinned ? pin.onUnpin : undefined}
						project={project}
						onEdit={onEdit && pickDialog(onEdit)}
						onArchive={onArchive && pickDialog(onArchive)}
						onUnarchive={onUnarchive}
						onDelete={pickDialog(onDelete)}
					/>
				</DropdownMenuContent>
			</DropdownMenu>
			{pin?.pinned && (
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="subtle"
							size="icon"
							aria-label={`Unpin ${project.name}`}
							className="group/pin size-7"
							onClick={() => {
								pin.onUnpin();
								// Unpinning removes this button, so focus moves to actions.
								actionsButtonRef.current?.focus();
							}}
						>
							<PinIcon className="group-hover/pin:hidden group-focus-visible/pin:hidden" />
							<PinOffIcon className="hidden group-hover/pin:block group-focus-visible/pin:block" />
						</Button>
					</TooltipTrigger>
					<TooltipContent side="bottom" align="end">
						Unpin project
					</TooltipContent>
				</Tooltip>
			)}
			{pin && !pin.pinned && (
				// Holds the pin's place, so a second click there after unpinning
				// does not open the actions menu.
				<span className="flex size-7 items-center justify-center">
					<Spinner loading={pin.pinPending} label={`Pinning ${project.name}`} />
				</span>
			)}
		</div>
	);
};

type ActivityLabelProps = {
	readonly summary: ChatProjectSummary;
};

const ActivityLabel: React.FC<ActivityLabelProps> = ({ summary }) => {
	const { own_chat_count, own_needs_attention, own_running, own_unread_count } =
		summary;
	let icon: React.ReactNode = null;
	let label = own_chat_count > 0 ? `${own_chat_count} read` : "No chats";
	if (own_needs_attention) {
		icon = <CircleAlertIcon className="text-highlight-orange" />;
		label = "Needs attention";
	} else if (own_running) {
		icon = <runningStatus.icon className={runningStatus.className} />;
		label = "Thinking";
	} else if (own_unread_count > 0) {
		icon = <span className="size-2 rounded-full bg-content-link" />;
		label = `${own_unread_count} unread`;
	}
	return (
		<span className="flex min-w-0 items-center gap-1">
			{icon && (
				<span aria-hidden="true" className="flex shrink-0 [&>svg]:size-3.5">
					{icon}
				</span>
			)}
			<span className="truncate">{label}</span>
		</span>
	);
};
