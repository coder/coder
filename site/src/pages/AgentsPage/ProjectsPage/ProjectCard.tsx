import { cn } from "cn";
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
import type { ChatProjectSummary } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { shortRelativeTime } from "#/utils/time";
import { ChatProjectIcon } from "../components/ChatProjectIcon";
import { ProjectActionsMenuItems } from "../components/ChatsSidebar/chats/ProjectActionsMenuItems";
import { getChatStatusDisplay } from "../components/ChatsSidebar/tree/statusConfig";

/** Receives the button that opened an action, to return focus to it. */
type ProjectCardAction = (returnFocusTo: HTMLElement | null) => void;

type ProjectCardProps = {
	readonly summary: ChatProjectSummary;
	readonly to: To;
	readonly onPin: () => void;
	readonly onUnpin: () => void;
	readonly onEdit: ProjectCardAction;
	readonly onDelete: ProjectCardAction;
};

const compactNumber = new Intl.NumberFormat("en-US", {
	notation: "compact",
	maximumFractionDigits: 1,
});
const fullNumber = new Intl.NumberFormat("en-US");
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
	onEdit,
	onDelete,
}) => {
	const { project, pinned, member_count, last_activity_at } = summary;
	const footerId = useId();
	const actionsButtonRef = useRef<HTMLButtonElement>(null);
	return (
		<div
			className={cn(
				"group relative flex min-w-0 flex-col gap-3 rounded-lg border border-solid border-border bg-surface-primary p-4",
				"hover:bg-surface-secondary has-[a:focus-visible]:ring-2 has-[a:focus-visible]:ring-content-link",
			)}
		>
			<div className="flex size-8 shrink-0 items-center justify-center rounded-md border border-solid border-border bg-surface-secondary">
				<ChatProjectIcon project={project} className="size-4" />
			</div>
			<div className="flex min-w-0 flex-col gap-1">
				<h2 className="m-0 truncate text-sm font-semibold">
					<Link
						to={to}
						aria-describedby={footerId}
						// The overlay makes the whole card the link's hit area.
						className="text-content-primary no-underline outline-none after:absolute after:inset-0 after:rounded-lg"
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
				className="mt-auto flex min-w-0 items-center gap-3 text-xs text-content-secondary"
			>
				<ActivityLabel summary={summary} />
				{member_count > 1 ? (
					<span className="flex shrink-0 items-center gap-1">
						<UsersIcon aria-hidden="true" className="size-3.5" />
						<span aria-hidden="true" className="tabular-nums">
							{compactNumber.format(member_count).toLowerCase()}
						</span>
						<span className="sr-only">{`${fullNumber.format(member_count)} members`}</span>
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
			{/* Follows the link in the DOM so the link comes first in the tab
			    order, and sits above the link's overlay to take its own clicks. */}
			<div className="absolute top-4 right-4 z-10 flex h-8 items-center gap-1">
				{pinned && (
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="subtle"
								size="icon"
								aria-label={`Unpin ${project.name}`}
								className="group/pin size-7 min-w-0 text-content-secondary"
								onClick={onUnpin}
							>
								<PinIcon className="group-hover/pin:hidden group-focus-visible/pin:hidden" />
								<PinOffIcon className="hidden group-hover/pin:block group-focus-visible/pin:block" />
							</Button>
						</TooltipTrigger>
						<TooltipContent>Unpin project</TooltipContent>
					</Tooltip>
				)}
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button
							ref={actionsButtonRef}
							variant="subtle"
							size="icon"
							aria-label={`Open project actions for ${project.name}`}
							className={cn(
								"size-7 min-w-0 opacity-0 [@media(hover:none)]:opacity-100",
								"group-hover:opacity-100 group-focus-within:opacity-100 data-[state=open]:opacity-100",
							)}
						>
							<EllipsisVerticalIcon />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						<ProjectActionsMenuItems
							Item={DropdownMenuItem}
							Separator={DropdownMenuSeparator}
							onPin={pinned ? undefined : onPin}
							onUnpin={pinned ? onUnpin : undefined}
							permissions={project.permissions}
							onEdit={() => onEdit(actionsButtonRef.current)}
							onDelete={() => onDelete(actionsButtonRef.current)}
						/>
					</DropdownMenuContent>
				</DropdownMenu>
			</div>
		</div>
	);
};

const ActivityLabel: React.FC<{ readonly summary: ChatProjectSummary }> = ({
	summary,
}) => {
	const { own_needs_attention, own_running, own_unread_count } = summary;
	const [icon, label] = own_needs_attention
		? [<CircleAlertIcon />, "Needs attention"]
		: own_running
			? [<runningStatus.icon className={runningStatus.className} />, "Thinking"]
			: own_unread_count > 0
				? [
						<span className="size-2 rounded-full bg-content-link" />,
						`${own_unread_count} unread`,
					]
				: [
						null,
						summary.own_chat_count > 0
							? `${summary.own_chat_count} read`
							: "No chats",
					];
	return (
		<span
			className={cn(
				"flex min-w-0 items-center gap-1",
				own_needs_attention && "text-content-warning",
			)}
		>
			{icon && (
				<span aria-hidden="true" className="flex shrink-0 [&>svg]:size-3.5">
					{icon}
				</span>
			)}
			<span className="truncate">{label}</span>
		</span>
	);
};
