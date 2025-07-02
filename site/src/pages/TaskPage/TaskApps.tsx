import type { WorkspaceApp } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { InfoTooltip } from "components/InfoTooltip/InfoTooltip";
import { ChevronDownIcon, LayoutGridIcon } from "lucide-react";
import { useAppLink } from "modules/apps/useAppLink";
import type { Task } from "modules/tasks/tasks";
import type React from "react";
import { type FC, useState } from "react";
import { Link as RouterLink } from "react-router-dom";
import { cn } from "utils/cn";
import { TaskAppIFrame } from "./TaskAppIframe";

type TaskAppsProps = {
	task: Task;
};

export const TaskApps: FC<TaskAppsProps> = ({ task }) => {
	const agents = task.workspace.latest_build.resources
		.flatMap((r) => r.agents)
		.filter((a) => !!a);

	// The Chat UI app will be displayed in the sidebar, so we don't want to show
	// it here
	const apps = agents
		.flatMap((a) => a?.apps)
		.filter(
			(a) => !!a && a.id !== task.workspace.latest_build.ai_task_sidebar_app_id,
		);

	const embeddedApps = apps.filter((app) => !app.external);
	const externalApps = apps.filter((app) => app.external);

	const [activeAppId, setActiveAppId] = useState<string>(() => {
		const appId = embeddedApps[0]?.id;
		if (!appId) {
			throw new Error("No apps found in task");
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

	return (
		<main className="flex flex-col">
			<div className="w-full flex items-center border-0 border-b border-border border-solid">
				<div className="p-2 pb-0 flex gap-2 items-center">
					{embeddedApps.map((app) => (
						<TaskAppTab
							key={app.id}
							task={task}
							app={app}
							active={app.id === activeAppId}
							onClick={(e) => {
								e.preventDefault();
								setActiveAppId(app.id);
							}}
						/>
					))}
				</div>

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
								{externalApps.map((app) => {
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

type TaskAppTabProps = {
	task: Task;
	app: WorkspaceApp;
	active: boolean;
	onClick: (e: React.MouseEvent<HTMLAnchorElement>) => void;
};

const TaskAppTab: FC<TaskAppTabProps> = ({ task, app, active, onClick }) => {
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
				"px-3",
				{
					"text-content-primary bg-surface-tertiary rounded-sm rounded-b-none":
						active,
				},
				{ "opacity-75 hover:opacity-100": !active },
			])}
		>
			<RouterLink to={link.href} onClick={onClick}>
				{app.icon ? <ExternalImage src={app.icon} /> : <LayoutGridIcon />}
				{link.label}
				{app.health === "unhealthy" && (
					<InfoTooltip
						title="This app is unhealthy."
						message="The health check failed."
						type="warning"
					/>
				)}
			</RouterLink>
		</Button>
	);
};
