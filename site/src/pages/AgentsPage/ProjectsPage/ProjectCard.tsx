import {
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
import { ProjectActionsMenuItems } from "../components/ChatsSidebar/chats/ProjectActionsMenuItems";
import { getChatStatusDisplay } from "../components/ChatsSidebar/tree/statusConfig";

type ProjectCardProps = {
	readonly summary: ChatProjectSummary;
	readonly to: To;
	readonly onPin: () => void;
	readonly onUnpin: () => void;
	/** A pin request for the project is running, so Pin is held back. */
	readonly pinPending: boolean;
	readonly onEdit: (returnFocusTo: HTMLElement | null) => void;
	readonly onDelete: (returnFocusTo: HTMLElement | null) => void;
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
	onDelete,
}) => {
	const { project, pinned, member_count, last_activity_at } = summary;
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
				{last_activity_at && (
					<time
						dateTime={last_activity_at}
						data-pixel="ignore"
						className="ml-auto shrink-0 tabular-nums"
					>
						{shortRelativeTime(last_activity_at)}
					</time>
				)}
			</div>
			{/* Follows the link in tab order. */}
			<ProjectCardActions
				project={project}
				pinned={pinned}
				pinPending={pinPending}
				onPin={onPin}
				onUnpin={onUnpin}
				onEdit={onEdit}
				onDelete={onDelete}
			/>
		</div>
	);
};

type ProjectCardActionsProps = Omit<ProjectCardProps, "summary" | "to"> & {
	readonly project: ChatProject;
	readonly pinned: boolean;
};

const ProjectCardActions: React.FC<ProjectCardActionsProps> = ({
	project,
	pinned,
	pinPending,
	onPin,
	onUnpin,
	onEdit,
	onDelete,
}) => {
	const actionsButtonRef = useRef<HTMLButtonElement>(null);
	// Edit and Delete open a dialog only once the menu has closed, so the
	// closing menu cannot take focus back from the dialog.
	const openPickedDialogRef = useRef<typeof onEdit>(undefined);
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
						onPin={pinned || pinPending ? undefined : onPin}
						onUnpin={pinned ? onUnpin : undefined}
						permissions={project.permissions}
						onEdit={() => {
							openPickedDialogRef.current = onEdit;
						}}
						onDelete={() => {
							openPickedDialogRef.current = onDelete;
						}}
					/>
				</DropdownMenuContent>
			</DropdownMenu>
			{pinned ? (
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="subtle"
							size="icon"
							aria-label={`Unpin ${project.name}`}
							className="group/pin size-7"
							onClick={() => {
								onUnpin();
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
			) : (
				// Holds the pin's place, so a second click there after unpinning
				// does not open the actions menu.
				<span className="flex size-7 items-center justify-center">
					<Spinner loading={pinPending} label={`Pinning ${project.name}`} />
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
