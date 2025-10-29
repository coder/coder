import type {
	WorkspaceAppStatus as APIWorkspaceAppStatus,
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import capitalize from "lodash/capitalize";
import {
	ChevronDownIcon,
	ChevronUpIcon,
	ExternalLinkIcon,
	FileIcon,
	LayoutGridIcon,
	SquareCheckBigIcon,
} from "lucide-react";
import { AppStatusStateIcon } from "modules/apps/AppStatusStateIcon";
import { useAppLink } from "modules/apps/useAppLink";
import { type FC, useState } from "react";
import { Link as RouterLink } from "react-router";
import { timeFrom } from "utils/time";
import { truncateURI } from "utils/uri";

interface AppStatusesProps {
	workspace: Workspace;
	agent: WorkspaceAgent;
	/** Optional reference date for calculating relative time. Defaults to Date.now(). Useful for Storybook. */
	referenceDate?: Date;
}

// Extend the API status type to include the app icon and the app itself
interface StatusWithAppInfo extends APIWorkspaceAppStatus {
	appIcon?: string; // Kept for potential future use, but we'll primarily use app.icon
	app?: WorkspaceApp; // Store the full app object
}

export const AppStatuses: FC<AppStatusesProps> = ({
	workspace,
	agent,
	referenceDate,
}) => {
	const [displayStatuses, setDisplayStatuses] = useState(false);
	const allStatuses: StatusWithAppInfo[] = agent.apps.flatMap((app) =>
		app.statuses
			.map((status) => ({
				...status,
				app,
			}))
			.sort(
				(a, b) =>
					new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
			),
	);

	if (allStatuses.length === 0) {
		return null;
	}

	const comparisonDate = referenceDate ?? new Date();
	const latestStatus = allStatuses[0];
	const otherStatuses = allStatuses.slice(1);

	return (
		<div className="flex flex-col border border-solid border-border rounded-lg">
			<div
				className={`
					flex items-center justify-between px-4 py-3 gap-6
					border-0 [&:not(:last-child)]:border-b border-solid border-border
				`}
			>
				<div className="flex flex-col overflow-hidden">
					<div className="text-sm font-medium text-content-primary flex items-center gap-2 ">
						<AppStatusStateIcon state={latestStatus.state} latest />
						<span className="block flex-1 whitespace-nowrap overflow-hidden text-ellipsis">
							{latestStatus.message || capitalize(latestStatus.state)}
						</span>
					</div>
					<time className="text-xs text-content-secondary first-letter:uppercase block pl-[26px]">
						{timeFrom(new Date(latestStatus.created_at), comparisonDate)}
					</time>
				</div>

				<div className="flex items-center gap-2">
					{latestStatus.app && (
						<AppLink
							app={latestStatus.app}
							agent={agent}
							workspace={workspace}
						/>
					)}

					{latestStatus.uri &&
						(latestStatus.uri.startsWith("file://") ? (
							<TooltipProvider>
								<Tooltip>
									<TooltipTrigger>
										<span className="flex items-center gap-1">
											<FileIcon className="size-icon-xs" />
											{truncateURI(latestStatus.uri)}
										</span>
									</TooltipTrigger>
									<TooltipContent>
										This file is located in your workspace
									</TooltipContent>
								</Tooltip>
							</TooltipProvider>
						) : (
							<Button asChild variant="outline" size="sm">
								<a href={latestStatus.uri} target="_blank" rel="noreferrer">
									<ExternalLinkIcon />
									{truncateURI(latestStatus.uri)}
								</a>
							</Button>
						))}

					{workspace.task_id && (
						<Button asChild size="sm" variant="outline">
							<RouterLink
								to={`/tasks/${workspace.owner_name}/${workspace.task_id}`}
							>
								<SquareCheckBigIcon />
								View task
							</RouterLink>
						</Button>
					)}

					<TooltipProvider>
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									disabled={otherStatuses.length === 0}
									size="icon"
									variant="subtle"
									onClick={() => {
										setDisplayStatuses((display) => !display);
									}}
								>
									{displayStatuses ? <ChevronUpIcon /> : <ChevronDownIcon />}
								</Button>
							</TooltipTrigger>
							<TooltipContent>
								{displayStatuses ? "Hide statuses" : "Show statuses"}
							</TooltipContent>
						</Tooltip>
					</TooltipProvider>
				</div>
			</div>

			{displayStatuses && (
				<ScrollArea className="h-[200px]">
					{otherStatuses.map((status) => {
						const statusTime = new Date(status.created_at);
						const formattedTimestamp = timeFrom(statusTime, comparisonDate);

						return (
							<div
								key={status.id}
								className={`
							flex items-center justify-between px-4 py-3
							border-0 [&:not(:last-child)]:border-b border-solid border-border
						`}
							>
								<div className="flex items-center justify-between w-full text-content-secondary">
									<span className="text-xs flex items-center gap-2">
										<AppStatusStateIcon
											state={status.state}
											latest={false}
											className="size-icon-xs w-[18px]"
										/>
										{status.message || capitalize(status.state)}
									</span>
									<span className="text-2xs text-content-secondary first-letter:uppercase block pl-[26px]">
										{formattedTimestamp}
									</span>
								</div>
							</div>
						);
					})}
				</ScrollArea>
			)}
		</div>
	);
};

type AppLinkProps = {
	app: WorkspaceApp;
	agent: WorkspaceAgent;
	workspace: Workspace;
};

const AppLink: FC<AppLinkProps> = ({ app, agent, workspace }) => {
	const link = useAppLink(app, { agent, workspace });

	return (
		<Button asChild variant="outline" size="sm">
			<a
				href={link.href}
				onClick={link.onClick}
				target="_blank"
				rel="noreferrer"
			>
				{app.icon ? <ExternalImage src={app.icon} /> : <LayoutGridIcon />}
				{link.label}
			</a>
		</Button>
	);
};
