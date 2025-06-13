import GitHub from "@mui/icons-material/GitHub";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import { Spinner } from "components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import {
	ArrowLeftIcon,
	BugIcon,
	EllipsisVerticalIcon,
	ExternalLinkIcon,
	GitPullRequestArrowIcon,
} from "lucide-react";
import { AppStatusStateIcon } from "modules/apps/AppStatusStateIcon";
import type { Task } from "modules/tasks/tasks";
import type { FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import { cn } from "utils/cn";
import { timeFrom } from "utils/time";
import { truncateURI } from "utils/uri";
import { TaskAppIFrame } from "./TaskAppIframe";
import { AI_APP_CHAT_SLUG, AI_APP_CHAT_URL_PATHNAME } from "./constants";

type TaskSidebarProps = {
	task: Task;
};

export const TaskSidebar: FC<TaskSidebarProps> = ({ task }) => {
	const chatApp = task.workspace.latest_build.resources
		.flatMap((r) => r.agents)
		.flatMap((a) => a?.apps)
		.find((a) => a?.slug === AI_APP_CHAT_SLUG);
	const showChatApp =
		chatApp && (chatApp.health === "disabled" || chatApp.health === "healthy");

	return (
		<aside
			className={cn([
				[
					"flex flex-col h-full shrink-0",
					"border-0 border-r border-solid border-border",
				],
				// We want to make the sidebar wider for chat apps
				showChatApp ? "w-[520px]" : "w-[320px]",
			])}
		>
			<header className="border-0 border-b border-solid border-border p-4 pt-0">
				<div className="flex items-center justify-between py-1">
					<TooltipProvider>
						<Tooltip>
							<TooltipTrigger asChild>
								<Button size="icon" variant="subtle" asChild className="-ml-2">
									<RouterLink to="/tasks">
										<ArrowLeftIcon />
										<span className="sr-only">Back to tasks</span>
									</RouterLink>
								</Button>
							</TooltipTrigger>
							<TooltipContent>Back to tasks</TooltipContent>
						</Tooltip>
					</TooltipProvider>

					<DropdownMenu>
						<TooltipProvider>
							<Tooltip>
								<TooltipTrigger asChild>
									<DropdownMenuTrigger asChild>
										<Button size="icon" variant="subtle" className="-mr-2">
											<EllipsisVerticalIcon />
											<span className="sr-only">Settings</span>
										</Button>
									</DropdownMenuTrigger>
								</TooltipTrigger>
								<TooltipContent>Settings</TooltipContent>
							</Tooltip>
						</TooltipProvider>

						<DropdownMenuContent>
							<DropdownMenuItem asChild>
								<RouterLink
									to={`/@${task.workspace.owner_name}/${task.workspace.name}`}
								>
									View workspace
								</RouterLink>
							</DropdownMenuItem>
						</DropdownMenuContent>
					</DropdownMenu>
				</div>

				<h1 className="m-0 mt-1 text-base font-medium truncate">
					{task.prompt}
				</h1>

				{task.workspace.latest_app_status?.uri && (
					<div className="flex items-center gap-2 mt-2 flex-wrap">
						<TaskStatusLink uri={task.workspace.latest_app_status.uri} />
					</div>
				)}
			</header>

			{showChatApp ? (
				<TaskAppIFrame
					active
					key={chatApp.id}
					app={chatApp}
					task={task}
					pathname={AI_APP_CHAT_URL_PATHNAME}
				/>
			) : (
				<TaskStatuses task={task} />
			)}
		</aside>
	);
};

type TaskStatusesProps = {
	task: Task;
};

const TaskStatuses: FC<TaskStatusesProps> = ({ task }) => {
	let statuses = task.workspace.latest_build.resources
		.flatMap((r) => r.agents)
		.flatMap((a) => a?.apps)
		.flatMap((a) => a?.statuses)
		.filter((s) => !!s)
		.sort(
			(a, b) =>
				new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
		);

	// This happens when the workspace is not running so it has no resources to
	// get the statuses so we can fallback to the latest status received from the
	// workspace.
	if (statuses.length === 0 && task.workspace.latest_app_status) {
		statuses = [task.workspace.latest_app_status];
	}

	return statuses ? (
		<ScrollArea className="h-full">
			{statuses.length === 0 && (
				<article className="px-4 py-2 flex gap-2 first-of-type:pt-4 last-of-type:pb-4">
					<div className="flex flex-col gap-1 flex-1">
						<h3 className="m-0 font-medium text-sm leading-normal">
							Running your task
						</h3>
						<time
							dateTime={task.workspace.latest_build.created_at}
							className="font-medium text-xs text-content-secondary first-letter:uppercase"
						>
							{timeFrom(new Date(task.workspace.latest_build.created_at))}
						</time>
					</div>

					<AppStatusStateIcon state="working" latest className="size-5" />
				</article>
			)}
			{statuses.map((status, index) => {
				return (
					<article
						className={cn(
							["px-4 py-2 flex gap-2 first-of-type:pt-4 last-of-type:pb-4"],
							{
								"opacity-50 hover:opacity-100": index !== 0,
							},
						)}
						key={status.id}
					>
						<div className="flex flex-col gap-1 flex-1">
							<h3 className="m-0 font-medium text-sm leading-normal">
								{status.message}
							</h3>
							<time
								dateTime={status.created_at}
								className="font-medium text-xs text-content-secondary first-letter:uppercase"
							>
								{timeFrom(new Date(status.created_at))}
							</time>
						</div>

						<AppStatusStateIcon
							state={status.state}
							latest={index === 0}
							disabled={task.workspace.latest_build.status !== "running"}
							className={cn(["size-5", { "opacity-0": index !== 0 }])}
						/>
					</article>
				);
			})}
		</ScrollArea>
	) : (
		<Spinner loading />
	);
};

type TaskStatusLinkProps = {
	uri: string;
};

const TaskStatusLink: FC<TaskStatusLinkProps> = ({ uri }) => {
	let icon = <ExternalLinkIcon />;
	let label = truncateURI(uri);

	if (uri.startsWith("https://github.com")) {
		const issueNumber = uri.split("/").pop();
		const [org, repo] = uri.split("/").slice(3, 5);
		const prefix = `${org}/${repo}`;

		if (uri.includes("pull/")) {
			icon = <GitPullRequestArrowIcon />;
			label = issueNumber
				? `${prefix}#${issueNumber}`
				: `${prefix} Pull Request`;
		} else if (uri.includes("issues/")) {
			icon = <BugIcon />;
			label = issueNumber ? `${prefix}#${issueNumber}` : `${prefix} Issue`;
		} else {
			icon = <GitHub />;
			label = `${org}/${repo}`;
		}
	}

	return (
		<Button asChild variant="outline" size="sm" className="min-w-0">
			<a href={uri} target="_blank" rel="noreferrer">
				{icon}
				{label}
			</a>
		</Button>
	);
};
