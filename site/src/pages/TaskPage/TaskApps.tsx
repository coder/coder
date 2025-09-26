import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { InfoTooltip } from "components/InfoTooltip/InfoTooltip";
import { Link } from "components/Link/Link";
import { ScrollArea, ScrollBar } from "components/ScrollArea/ScrollArea";
import { ChevronDownIcon, LayoutGridIcon } from "lucide-react";
import { useAppLink } from "modules/apps/useAppLink";
import {
	getTaskApps,
	type Task,
	type WorkspaceAppWithAgent,
} from "modules/tasks/tasks";
import type React from "react";
import { type FC, useState } from "react";
import { Link as RouterLink } from "react-router";
import { cn } from "utils/cn";
import { docs } from "utils/docs";
import { TaskAppIFrame } from "./TaskAppIframe";

type TaskAppsProps = {
	task: Task;
};

export const TaskApps: FC<TaskAppsProps> = ({ task }) => {
	const apps = getTaskApps(task).filter(
		// The Chat UI app will be displayed in the sidebar, so we don't want to
		// show it as a web app.
		(app) =>
			app.id !== task.workspace.latest_build.ai_task_sidebar_app_id &&
			app.health !== "disabled",
	);
	const [embeddedApps, externalApps] = splitEmbeddedAndExternalApps(apps);
	const [activeAppId, setActiveAppId] = useState(embeddedApps.at(0)?.id);
	const hasAvailableAppsToDisplay =
		embeddedApps.length > 0 || externalApps.length > 0;

	return (
		<main className="flex flex-col h-full">
			{hasAvailableAppsToDisplay && (
				<div className="w-full flex items-center border-0 border-b border-border border-solid">
					<ScrollArea className="max-w-full">
						<div className="flex w-max gap-2 items-center p-2 pb-0">
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
						<ScrollBar orientation="horizontal" className="h-2" />
					</ScrollArea>

					{externalApps.length > 0 && (
						<ExternalAppsDropdown task={task} externalApps={externalApps} />
					)}
				</div>
			)}

			{embeddedApps.length > 0 ? (
				<div className="flex-1">
					{embeddedApps.map((app) => (
						<TaskAppIFrame
							key={app.id}
							active={activeAppId === app.id}
							app={app}
							task={task}
						/>
					))}
				</div>
			) : (
				<div className="mx-auto my-auto flex flex-col items-center">
					<h3 className="font-medium text-content-primary text-base">
						No embedded apps found.
					</h3>

					<span className="text-content-secondary text-sm">
						<Link
							href={docs("/ai-coder/tasks")}
							target="_blank"
							rel="noreferrer"
						>
							Learn how to configure apps
						</Link>{" "}
						for your tasks.
					</span>
				</div>
			)}
		</main>
	);
};

type ExternalAppsDropdownProps = {
	task: Task;
	externalApps: WorkspaceAppWithAgent[];
};

const ExternalAppsDropdown: FC<ExternalAppsDropdownProps> = ({
	task,
	externalApps,
}) => {
	return (
		<div className="ml-auto">
			<DropdownMenu>
				<DropdownMenuTrigger asChild>
					<Button size="sm" variant="subtle">
						Open locally
						<ChevronDownIcon />
					</Button>
				</DropdownMenuTrigger>
				<DropdownMenuContent>
					{externalApps.map((app) => (
						<ExternalAppMenuItem key={app.id} app={app} task={task} />
					))}
				</DropdownMenuContent>
			</DropdownMenu>
		</div>
	);
};

const ExternalAppMenuItem: FC<{
	app: WorkspaceAppWithAgent;
	task: Task;
}> = ({ app, task }) => {
	const link = useAppLink(app, {
		agent: app.agent,
		workspace: task.workspace,
	});

	return (
		<DropdownMenuItem asChild>
			<RouterLink to={link.href}>
				{app.icon ? <ExternalImage src={app.icon} /> : <LayoutGridIcon />}
				{link.label}
			</RouterLink>
		</DropdownMenuItem>
	);
};

type TaskAppTabProps = {
	task: Task;
	app: WorkspaceAppWithAgent;
	active: boolean;
	onClick: (e: React.MouseEvent<HTMLAnchorElement>) => void;
};

const TaskAppTab: FC<TaskAppTabProps> = ({ task, app, active, onClick }) => {
	const link = useAppLink(app, {
		agent: app.agent,
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

function splitEmbeddedAndExternalApps(
	apps: WorkspaceAppWithAgent[],
): [WorkspaceAppWithAgent[], WorkspaceAppWithAgent[]] {
	const embeddedApps = [];
	const externalApps = [];

	for (const app of apps) {
		if (app.external) {
			externalApps.push(app);
		} else {
			embeddedApps.push(app);
		}
	}

	return [embeddedApps, externalApps];
}
