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
import { useProxy } from "contexts/ProxyContext";
import {
	ArrowLeftIcon,
	ChevronDownIcon,
	LayoutGridIcon,
	RotateCcwIcon,
} from "lucide-react";
import { AppStatusIcon } from "modules/apps/AppStatusIcon";
import { getAppHref } from "modules/apps/apps";
import { useAppLink } from "modules/apps/useAppLink";
import { AI_PROMPT_PARAMETER_NAME, type Task } from "modules/tasks/tasks";
import { WorkspaceAppStatus } from "modules/workspaces/WorkspaceAppStatus/WorkspaceAppStatus";
import type React from "react";
import { type FC, type ReactNode, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import { Link as RouterLink } from "react-router-dom";
import { cn } from "utils/cn";
import { pageTitle } from "utils/page";
import { timeFrom } from "utils/time";

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
						Building your task
					</h3>
					<span className="text-content-secondary text-sm">
						Your task is being built and will be ready soon
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
							to={`/@${task.workspace.owner_username}/${task.workspace.name}/builds/${task.workspace.latest_build.build_number}`}
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
				<div className="py-6 flex flex-col gap-3">
					{task.workspace.latest_app_status && (
						<div className="p-3 border border-border border-solid rounded-lg">
							<WorkspaceAppStatus status={task.workspace.latest_app_status} />
						</div>
					)}
					<div className="border border-border border-solid rounded-lg w-full min-h-80 flex items-center justify-center">
						<div className="flex flex-col items-center">
							<h3 className="m-0 font-medium text-content-primary text-base">
								Task build terminated
							</h3>
							<span className="text-content-secondary text-sm">
								So apps and previous statuses are not available
							</span>
						</div>
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
		const statuses = task.workspace.latest_build.resources
			.flatMap((r) => r.agents)
			.flatMap((a) => a?.apps)
			.flatMap((a) => a?.statuses)
			.filter((s) => !!s)
			.sort(
				(a, b) =>
					new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
			);

		content = (
			<div className="flex-1 flex justify-stretch overflow-hidden">
				<aside className="w-full max-w-xs border-0 border-r border-border border-solid">
					<ScrollArea className="h-full py-3">
						{statuses.map((status, index) => {
							return (
								<article
									className={cn(["px-4 py-3 flex gap-3"], {
										"opacity-75 hover:opacity-100": index !== 0,
									})}
									key={status.id}
								>
									<AppStatusIcon
										status={status}
										latest={index === 0}
										className="size-4 mt-1"
									/>
									<div className="flex flex-col gap-1">
										<h3 className="m-0 font-medium text-sm">
											{status.message}
										</h3>
										<time
											dateTime={status.created_at}
											className="font-medium text-xs text-content-secondary"
										>
											{timeFrom(new Date(status.created_at))}
										</time>
									</div>
								</article>
							);
						})}
					</ScrollArea>
				</aside>

				<TaskApps task={task} />
			</div>
		);
	}

	return (
		<>
			<Helmet>
				<title>{pageTitle(task.prompt)}</title>
			</Helmet>

			<section className="h-full flex flex-col">
				<header className="h-20 border-0 border-b border-solid border-border px-4 flex items-center shrink-0">
					<div className="flex items-center gap-4">
						<TooltipProvider>
							<Tooltip>
								<TooltipTrigger asChild>
									<Button size="icon-lg" variant="outline" asChild>
										<RouterLink to="/tasks">
											<ArrowLeftIcon />
											<span className="sr-only">Back to tasks</span>
										</RouterLink>
									</Button>
								</TooltipTrigger>
								<TooltipContent>Back to tasks</TooltipContent>
							</Tooltip>
						</TooltipProvider>

						<div className="flex flex-col">
							<h1 className="m-0 text-sm font-medium">{task.prompt}</h1>
							<span className="text-xs text-content-secondary">
								Created by{" "}
								{task.workspace.owner_name ?? task.workspace.owner_username}{" "}
								{timeFrom(new Date(task.workspace.created_at))}
							</span>
						</div>
					</div>
				</header>

				{content}
			</section>
		</>
	);
};

export default TaskPage;

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

	const { proxy } = useProxy();
	const [iframeSrc, setIframeSrc] = useState(() => {
		const src = getAppHref(activeApp, {
			agent,
			workspace: task.workspace,
			path: proxy.preferredPathAppURL,
			host: proxy.preferredWildcardHostname,
		});
		return src;
	});

	return (
		<main className="flex-1 flex flex-col">
			<div className="border-0 border-b border-border border-solid w-full p-1 flex gap-2">
				{apps
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
								setIframeSrc(e.currentTarget.href);
							}}
						/>
					))}

				<div className="ml-auto">
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button size="sm" variant="subtle">
								Open in IDE
								<ChevronDownIcon />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent>
							{apps
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
			</div>

			<div className="flex-1">
				<iframe
					title={activeApp.display_name ?? activeApp.slug}
					className="w-full h-full border-0"
					src={iframeSrc}
				/>
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
