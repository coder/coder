import type {
	WorkspaceAppStatus as APIWorkspaceAppStatus,
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Spinner } from "components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { formatDistance } from "date-fns";
import {
	ChevronDownIcon,
	ChevronUpIcon,
	CircleAlertIcon,
	CircleCheckIcon,
	ExternalLinkIcon,
	FileIcon,
	HourglassIcon,
	LayoutGridIcon,
	TriangleAlertIcon,
} from "lucide-react";
import { useAppLink } from "modules/apps/useAppLink";
import { type FC, useState } from "react";
import { cn } from "utils/cn";

const getStatusColor = (state: APIWorkspaceAppStatus["state"]) => {
	switch (state) {
		case "complete":
			return "text-content-success";
		case "failure":
			return "text-content-warning";
		case "working":
			return "text-highlight-sky";
		default:
			return "text-content-secondary";
	}
};

const getStatusIcon = (
	state: APIWorkspaceAppStatus["state"],
	isLatest: boolean,
	className?: string,
) => {
	const iconClassName = cn(["size-[18px]", getStatusColor(state), className]);

	switch (state) {
		case "complete":
			return <CircleCheckIcon className={iconClassName} />;
		case "failure":
			return <CircleAlertIcon className={iconClassName} />;
		case "working":
			return isLatest ? (
				<Spinner size="sm" loading />
			) : (
				<HourglassIcon className={iconClassName} />
			);
		default:
			return <TriangleAlertIcon className={iconClassName} />;
	}
};

const formatURI = (uri: string) => {
	if (uri.startsWith("file://")) {
		const path = uri.slice(7);
		// Slightly shorter truncation for this context if needed
		if (path.length > 35) {
			const start = path.slice(0, 15);
			const end = path.slice(-15);
			return `${start}...${end}`;
		}
		return path;
	}

	try {
		const url = new URL(uri);
		const fullUrl = url.toString();
		// Slightly shorter truncation
		if (fullUrl.length > 40) {
			const start = fullUrl.slice(0, 20);
			const end = fullUrl.slice(-20);
			return `${start}...${end}`;
		}
		return fullUrl;
	} catch {
		// Slightly shorter truncation
		if (uri.length > 35) {
			const start = uri.slice(0, 15);
			const end = uri.slice(-15);
			return `${start}...${end}`;
		}
		return uri;
	}
};

// --- Component Implementation ---

export interface AppStatusesProps {
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
					flex items-center justify-between px-4 py-3
					border-0 [&:not(:last-child)]:border-b border-solid border-border
				`}
			>
				<div className="flex flex-col">
					<span className="text-sm font-medium text-content-primary flex items-center gap-2">
						{getStatusIcon(latestStatus.state, true)}
						{latestStatus.message}
					</span>
					<span className="text-xs text-content-secondary first-letter:uppercase block pl-[26px]">
						{formatDistance(new Date(latestStatus.created_at), comparisonDate, {
							addSuffix: true,
						})}
					</span>
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
											{formatURI(latestStatus.uri)}
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
									{formatURI(latestStatus.uri)}
								</a>
							</Button>
						))}

					<TooltipProvider>
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
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

			{displayStatuses &&
				otherStatuses.map((status) => {
					const statusTime = new Date(status.created_at);
					const formattedTimestamp = formatDistance(
						statusTime,
						comparisonDate,
						{
							addSuffix: true,
						},
					);

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
									{getStatusIcon(status.state, false, "size-icon-xs w-[18px]")}
									{status.message}
								</span>
								<span className="text-2xs text-content-secondary first-letter:uppercase block pl-[26px]">
									{formattedTimestamp}
								</span>
							</div>
						</div>
					);
				})}
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
