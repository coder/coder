import type { WorkspaceAgent, WorkspaceApp } from "api/typesGenerated";
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
import {
	ChevronDownIcon,
	LayoutGridIcon,
	MoreHorizontalIcon,
} from "lucide-react";
import { useAppLink } from "modules/apps/useAppLink";
import type { Task } from "modules/tasks/tasks";
import type React from "react";
import { type FC, useLayoutEffect, useRef, useState } from "react";
import { Link as RouterLink } from "react-router";
import { cn } from "utils/cn";
import { docs } from "utils/docs";
import { getAgents } from "utils/workspace";
import { TaskAppIFrame } from "./TaskAppIframe";

type TaskAppsProps = {
	task: Task;
};

type WorkspaceAppWithAgent = WorkspaceApp & {
	agent: WorkspaceAgent;
};

export const TaskApps: FC<TaskAppsProps> = ({ task }) => {
	const agents = getAgents(task.workspace);
	const apps: WorkspaceAppWithAgent[] = agents
		.flatMap((agent) =>
			agent.apps.map((app) => ({
				...app,
				agent,
			})),
		)
		// The Chat UI app will be displayed in the sidebar, so we don't want to
		// show it as a tab.
		.filter(
			(app) => app.id !== task.workspace.latest_build.ai_task_sidebar_app_id,
		);
	const embedApps = apps.filter((app) => !app.external);
	const externalApps = apps.filter((app) => app.external);
	const [activeAppId, setActiveAppId] = useState(embedApps.at(0)?.id);

	// An agent can have many apps and they might not fit in the available
	// horizontal space. We hide the overflowing apps and show them in a dropdown
	// instead.
	const [visibleApps, setVisibleApps] =
		useState<WorkspaceAppWithAgent[]>(embedApps);
	const hiddenApps = embedApps.filter(
		(app) => !visibleApps.map((app) => app.id).includes(app.id),
	);
	const containerRef = useRef<HTMLDivElement>(null);
	// Adding this ignore was necessary because embedApps are being built on each
	// render, which would cause an infinite loop.
	useLayoutEffect(() => {
		const container = containerRef.current;
		if (container) {
			setVisibleApps((apps) => hideOverflowingApps(apps, container));
		}
	}, []);

	return (
		<main className="flex flex-col">
			<div className="w-full flex items-center border-0 border-b border-border border-solid">
				<div
					ref={containerRef}
					className="flex-1 p-2 pb-0 flex gap-2 items-center overflow-hidden"
				>
					{visibleApps.map((app) => (
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

				<div className="flex items-center gap-2 p-2 pb-0">
					{hiddenApps.length > 0 && (
						<MoreAppsDropdown
							task={task}
							apps={hiddenApps}
							onSelect={setActiveAppId}
						/>
					)}

					{externalApps.length > 0 && (
						<ExternalAppsDropdown
							task={task}
							agents={agents}
							externalApps={externalApps}
						/>
					)}
				</div>
			</div>

			{embedApps.length > 0 ? (
				<div className="flex-1">
					{embedApps.map((app) => {
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

type MoreAppsDropdownProps = {
	task: Task;
	apps: WorkspaceAppWithAgent[];
	onSelect: (appId: string) => void;
};

const MoreAppsDropdown: FC<MoreAppsDropdownProps> = ({
	task,
	apps,
	onSelect,
}) => {
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button size="icon" variant="subtle" title="More apps">
					<MoreHorizontalIcon />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent>
				{apps.map((app) => (
					<WebAppMenuItem
						key={app.id}
						app={app}
						task={task}
						onSelect={() => onSelect(app.id)}
					/>
				))}
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

type WebAppMenuItemProps = {
	app: WorkspaceAppWithAgent;
	task: Task;
	onSelect: () => void;
};

const WebAppMenuItem: FC<WebAppMenuItemProps> = ({ app, task, onSelect }) => {
	const link = useAppLink(app, {
		agent: app.agent,
		workspace: task.workspace,
	});

	return (
		<DropdownMenuItem onClick={onSelect}>
			{app.icon ? <ExternalImage src={app.icon} /> : <LayoutGridIcon />}
			{link.label}
			{app.health === "unhealthy" && (
				<InfoTooltip
					title="This app is unhealthy."
					message="The health check failed."
					type="warning"
				/>
			)}
		</DropdownMenuItem>
	);
};

type ExternalAppsDropdownProps = {
	task: Task;
	agents: WorkspaceAgent[];
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
			<RouterLink to={link.href} onClick={onClick} data-app-tab>
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

function hideOverflowingApps(
	apps: WorkspaceAppWithAgent[],
	container: HTMLDivElement,
) {
	const isOverflowing = container.scrollWidth > container.clientWidth;
	if (!isOverflowing) {
		return apps;
	}

	const tabs = container.querySelectorAll<HTMLAnchorElement>("[data-app-tab]");
	const gap = Number.parseInt(window.getComputedStyle(container).gap, 10);
	const visibleApps: WorkspaceAppWithAgent[] = [];
	let filledSpace = 0;

	for (const [index, tab] of tabs.entries()) {
		filledSpace += tab.offsetWidth + gap;
		if (filledSpace > container.clientWidth) {
			break;
		}
		visibleApps.push(apps[index]);
	}

	return visibleApps;
}
