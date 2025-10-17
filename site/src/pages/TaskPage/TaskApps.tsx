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
import { Terminal } from "components/Terminal/Terminal";
import { ChevronDownIcon, LayoutGridIcon, TerminalIcon } from "lucide-react";
import { useAppLink } from "modules/apps/useAppLink";
import {
	getTaskApps,
	type Task,
	type WorkspaceAppWithAgent,
} from "modules/tasks/tasks";
import type React from "react";
import { type FC, useMemo, useState } from "react";
import { Link as RouterLink } from "react-router";
import { cn } from "utils/cn";
import { docs } from "utils/docs";
import { v4 as uuidv4 } from "uuid";
import { TaskAppIFrame } from "./TaskAppIframe";

const TERMINAL_TAB_ID = "__terminal__";

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

	// Default to first embedded app, or terminal if no apps exist
	const defaultTab = embeddedApps.at(0)?.id ?? TERMINAL_TAB_ID;
	const [activeAppId, setActiveAppId] = useState(defaultTab);

	const hasAppsToDisplay = embeddedApps.length > 0 || externalApps.length > 0;
	const agent = task.workspace.latest_build.resources
		.flatMap((r) => r.agents ?? [])
		.filter((a) => !!a)
		.at(0);

	// Generate a stable reconnection token for the terminal session.
	// This token is stored in sessionStorage to persist across component remounts
	// but is cleared when the browser tab/window is closed.
	const terminalReconnectToken = useMemo(() => {
		if (!agent) return "";
		const storageKey = `terminal-reconnect-${agent.id}`;
		const stored = sessionStorage.getItem(storageKey);
		if (stored) return stored;
		const newToken = uuidv4();
		sessionStorage.setItem(storageKey, newToken);
		return newToken;
	}, [agent]);

	return (
		<main className="flex flex-col h-full">
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
						{agent && (
							<TerminalTab
								active={activeAppId === TERMINAL_TAB_ID}
								onClick={(e) => {
									e.preventDefault();
									setActiveAppId(TERMINAL_TAB_ID);
								}}
							/>
						)}
					</div>
					<ScrollBar orientation="horizontal" className="h-2" />
				</ScrollArea>

				{externalApps.length > 0 && (
					<ExternalAppsDropdown task={task} externalApps={externalApps} />
				)}
			</div>

			{hasAppsToDisplay || agent ? (
				<div className="flex-1">
					{embeddedApps.map((app) => (
						<TaskAppIFrame
							key={app.id}
							active={activeAppId === app.id}
							app={app}
							task={task}
						/>
					))}
					{agent && activeAppId === TERMINAL_TAB_ID && (
						<Terminal
							agentId={agent.id}
							agentName={agent.name}
							agentOS={agent.operating_system}
							workspaceName={task.workspace.name}
							username={task.workspace.owner_name}
							reconnectionToken={terminalReconnectToken}
						/>
					)}
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

type TerminalTabProps = {
	active: boolean;
	onClick: (e: React.MouseEvent<HTMLButtonElement>) => void;
};

const TerminalTab: FC<TerminalTabProps> = ({ active, onClick }) => {
	return (
		<Button
			size="sm"
			variant="subtle"
			className={cn([
				"px-3",
				{
					"text-content-primary bg-surface-tertiary rounded-sm rounded-b-none":
						active,
				},
				{ "opacity-75 hover:opacity-100": !active },
			])}
			onClick={onClick}
		>
			<TerminalIcon />
			Terminal
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
