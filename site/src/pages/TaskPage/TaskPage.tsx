import GitHub from "@mui/icons-material/GitHub";
import { API } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import type { WorkspaceApp, WorkspaceStatus } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
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
	ChevronDownIcon,
	EllipsisVerticalIcon,
	ExternalLinkIcon,
	GitPullRequestArrowIcon,
	LayoutGridIcon,
	RotateCcwIcon,
} from "lucide-react";
import { AppStatusStateIcon } from "modules/apps/AppStatusStateIcon";
import { useAppLink } from "modules/apps/useAppLink";
import { AI_PROMPT_PARAMETER_NAME, type Task } from "modules/tasks/tasks";
import type React from "react";
import { type FC, type ReactNode, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import { Link as RouterLink } from "react-router-dom";
import { cn } from "utils/cn";
import { pageTitle } from "utils/page";
import { timeFrom } from "utils/time";
import { truncateURI } from "utils/uri";

const TaskPage = () => {
	const { workspace: workspaceName, username } = useParams() as {
		workspace: string;
		username: string;
	};
	const {
		data: task,
		error,
		refetch,
	} = useQuery({
		queryKey: ["tasks", username, workspaceName],
		queryFn: () => data.fetchTask(username, workspaceName),
		refetchInterval: 5_000,
	});

	if (error) {
		return (
			<>
				<Helmet>
					<title>{pageTitle("Error loading task")}</title>
				</Helmet>

				<div className="w-full min-h-80 flex items-center justify-center">
					<div className="flex flex-col items-center">
						<h3 className="m-0 font-medium text-content-primary text-base">
							{getErrorMessage(error, "Failed to load task")}
						</h3>
						<span className="text-content-secondary text-sm">
							{getErrorDetail(error)}
						</span>
						<div className="mt-4 flex items-center gap-2">
							<Button size="sm" variant="outline" asChild>
								<RouterLink to="/tasks">
									<ArrowLeftIcon />
									Back to tasks
								</RouterLink>
							</Button>
							<Button size="sm" onClick={() => refetch()}>
								<RotateCcwIcon />
								Try again
							</Button>
						</div>
					</div>
				</div>
			</>
		);
	}

	if (!task) {
		return (
			<>
				<Helmet>
					<title>{pageTitle("Loading task")}</title>
				</Helmet>
				<Loader fullscreen />
			</>
		);
	}

	let content: ReactNode = null;
	const waitingStatuses: WorkspaceStatus[] = ["starting", "pending"];
	const terminatedStatuses: WorkspaceStatus[] = [
		"canceled",
		"canceling",
		"deleted",
		"deleting",
		"stopped",
		"stopping",
	];

	if (waitingStatuses.includes(task.workspace.latest_build.status)) {
		content = (
			<div className="w-full min-h-80 flex items-center justify-center">
				<div className="flex flex-col items-center">
					<Spinner loading className="mb-4" />
					<h3 className="m-0 font-medium text-content-primary text-base">
						Building the workspace
					</h3>
					<span className="text-content-secondary text-sm">
						Your task will run as soon as the workspace is ready
					</span>
				</div>
			</div>
		);
	} else if (task.workspace.latest_build.status === "failed") {
		content = (
			<div className="w-full min-h-80 flex items-center justify-center">
				<div className="flex flex-col items-center">
					<h3 className="m-0 font-medium text-content-primary text-base">
						Task build failed
					</h3>
					<span className="text-content-secondary text-sm">
						Please check the logs for more details.
					</span>
					<Button size="sm" variant="outline" asChild className="mt-4">
						<RouterLink
							to={`/@${task.workspace.owner_name}/${task.workspace.name}/builds/${task.workspace.latest_build.build_number}`}
						>
							View logs
						</RouterLink>
					</Button>
				</div>
			</div>
		);
	} else if (terminatedStatuses.includes(task.workspace.latest_build.status)) {
		content = (
			<Margins>
				<div className="w-full min-h-80 flex items-center justify-center">
					<div className="flex flex-col items-center">
						<h3 className="m-0 font-medium text-content-primary text-base">
							Workspace is not running
						</h3>
						<span className="text-content-secondary text-sm">
							Apps and previous statuses are not available
						</span>
						<Button size="sm" className="mt-4" asChild>
							<RouterLink
								to={`/@${task.workspace.owner_name}/${task.workspace.name}`}
							>
								View workspace
							</RouterLink>
						</Button>
					</div>
				</div>
			</Margins>
		);
	} else if (!task.workspace.latest_app_status) {
		content = (
			<div className="w-full min-h-80 flex items-center justify-center">
				<div className="flex flex-col items-center">
					<Spinner loading className="mb-4" />
					<h3 className="m-0 font-medium text-content-primary text-base">
						Running your task
					</h3>
					<span className="text-content-secondary text-sm">
						The status should be available soon
					</span>
				</div>
			</div>
		);
	} else {
		content = <TaskApps task={task} />;
	}

	return (
		<>
			<Helmet>
				<title>{pageTitle(task.prompt)}</title>
			</Helmet>

			<div className="h-full flex justify-stretch">
				<TaskSidebar task={task} />
				{content}
			</div>
		</>
	);
};

export default TaskPage;

type TaskSidebarProps = {
	task: Task;
};

const TaskSidebar: FC<TaskSidebarProps> = ({ task }) => {
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

	return (
		<aside className="flex flex-col h-full border-0 border-r border-solid border-border w-[320px] shrink-0">
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

				<h1 className="m-0 mt-1 text-base font-medium">{task.prompt}</h1>

				{task.workspace.latest_app_status?.uri && (
					<div className="flex items-center gap-2 mt-2 flex-wrap">
						<TaskStatusLink uri={task.workspace.latest_app_status.uri} />
					</div>
				)}
			</header>

			{statuses ? (
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
			)}
		</aside>
	);
};

type TaskAppsProps = {
	task: Task;
};

const TaskApps: FC<TaskAppsProps> = ({ task }) => {
	const agents = task.workspace.latest_build.resources
		.flatMap((r) => r.agents)
		.filter((a) => !!a);

	const apps = agents.flatMap((a) => a?.apps).filter((a) => !!a);

	const [activeAppId, setActiveAppId] = useState<string>(() => {
		const appId = task.workspace.latest_app_status?.app_id;
		if (!appId) {
			throw new Error("No active app found in task");
		}
		return appId;
	});

	const activeApp = apps.find((app) => app.id === activeAppId);
	if (!activeApp) {
		throw new Error(`Active app with ID ${activeAppId} not found in task`);
	}

	const agent = agents.find((a) =>
		a.apps.some((app) => app.id === activeAppId),
	);
	if (!agent) {
		throw new Error(`Agent for app ${activeAppId} not found in task workspace`);
	}

	const embeddedApps = apps.filter((app) => !app.external);
	const externalApps = apps.filter((app) => app.external);

	return (
		<main className="flex-1 flex flex-col">
			<div className="border-0 border-b border-border border-solid w-full p-1 flex gap-2">
				{embeddedApps
					.filter((app) => !app.external)
					.map((app) => (
						<TaskAppButton
							key={app.id}
							task={task}
							app={app}
							active={app.id === activeAppId}
							onClick={(e) => {
								if (app.external) {
									return;
								}

								e.preventDefault();
								setActiveAppId(app.id);
							}}
						/>
					))}

				{externalApps.length > 0 && (
					<div className="ml-auto">
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button size="sm" variant="subtle">
									Open locally
									<ChevronDownIcon />
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent>
								{externalApps
									.filter((app) => app.external)
									.map((app) => {
										const link = useAppLink(app, {
											agent,
											workspace: task.workspace,
										});

										return (
											<DropdownMenuItem key={app.id} asChild>
												<RouterLink to={link.href}>
													{app.icon ? (
														<ExternalImage src={app.icon} />
													) : (
														<LayoutGridIcon />
													)}
													{link.label}
												</RouterLink>
											</DropdownMenuItem>
										);
									})}
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
				)}
			</div>

			<div className="flex-1">
				{embeddedApps.map((app) => {
					return (
						<TaskAppIFrame
							key={app.id}
							active={activeAppId === app.id}
							app={app}
							task={task}
						/>
					);
				})}
			</div>
		</main>
	);
};

type TaskAppButtonProps = {
	task: Task;
	app: WorkspaceApp;
	active: boolean;
	onClick: (e: React.MouseEvent<HTMLAnchorElement>) => void;
};

const TaskAppButton: FC<TaskAppButtonProps> = ({
	task,
	app,
	active,
	onClick,
}) => {
	const agent = task.workspace.latest_build.resources
		.flatMap((r) => r.agents)
		.filter((a) => !!a)
		.find((a) => a.apps.some((a) => a.id === app.id));

	if (!agent) {
		throw new Error(`Agent for app ${app.id} not found in task workspace`);
	}

	const link = useAppLink(app, {
		agent,
		workspace: task.workspace,
	});

	return (
		<Button
			size="sm"
			variant="subtle"
			key={app.id}
			asChild
			className={cn([
				{ "text-content-primary": active },
				{ "opacity-75 hover:opacity-100": !active },
			])}
		>
			<RouterLink to={link.href} onClick={onClick}>
				{app.icon ? <ExternalImage src={app.icon} /> : <LayoutGridIcon />}
				{link.label}
			</RouterLink>
		</Button>
	);
};

type TaskAppIFrameProps = {
	task: Task;
	app: WorkspaceApp;
	active: boolean;
};

const TaskAppIFrame: FC<TaskAppIFrameProps> = ({ task, app, active }) => {
	const agent = task.workspace.latest_build.resources
		.flatMap((r) => r.agents)
		.filter((a) => !!a)
		.find((a) => a.apps.some((a) => a.id === app.id));

	if (!agent) {
		throw new Error(`Agent for app ${app.id} not found in task workspace`);
	}

	const link = useAppLink(app, {
		agent,
		workspace: task.workspace,
	});

	return (
		<iframe
			src={link.href}
			title={link.label}
			loading="eager"
			className={cn([active ? "block" : "hidden", "w-full h-full border-0"])}
		/>
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

export const data = {
	fetchTask: async (workspaceOwnerUsername: string, workspaceName: string) => {
		const workspace = await API.getWorkspaceByOwnerAndName(
			workspaceOwnerUsername,
			workspaceName,
		);
		const parameters = await API.getWorkspaceBuildParameters(
			workspace.latest_build.id,
		);
		const prompt = parameters.find(
			(p) => p.name === AI_PROMPT_PARAMETER_NAME,
		)?.value;

		if (!prompt) {
			return;
		}

		return {
			workspace,
			prompt,
		} satisfies Task;
	},
};
