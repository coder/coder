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
import { formatDistance, formatDistanceToNow } from "date-fns";
import {
	CircleAlertIcon,
	CircleCheckIcon,
	ExternalLinkIcon,
	FileIcon,
	HourglassIcon,
	TriangleAlertIcon,
} from "lucide-react";
import { useAppLink } from "modules/apps/useAppLink";
import type { FC } from "react";
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
) => {
	const className = cn(["size-[18px]", getStatusColor(state)]);

	switch (state) {
		case "complete":
			return <CircleCheckIcon className={className} />;
		case "failure":
			return <CircleAlertIcon className={className} />;
		case "working":
			return isLatest ? (
				<Spinner size="sm" loading />
			) : (
				<HourglassIcon className={className} />
			);
		default:
			return <TriangleAlertIcon className={className} />;
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
	apps: WorkspaceApp[];
	workspace: Workspace;
	agents: ReadonlyArray<WorkspaceAgent>;
	/** Optional reference date for calculating relative time. Defaults to Date.now(). Useful for Storybook. */
	referenceDate?: Date;
}

// Extend the API status type to include the app icon and the app itself
interface StatusWithAppInfo extends APIWorkspaceAppStatus {
	appIcon?: string; // Kept for potential future use, but we'll primarily use app.icon
	app?: WorkspaceApp; // Store the full app object
}

export const AppStatuses: FC<AppStatusesProps> = ({
	apps,
	workspace,
	agents,
	referenceDate,
}) => {
	// 1. Flatten all statuses and include the parent app object
	const allStatuses: StatusWithAppInfo[] = apps.flatMap((app) =>
		app.statuses.map((status) => ({
			...status,
			app: app, // Store the parent app object
		})),
	);

	// 2. Sort statuses chronologically (newest first) - mutating the value is
	// fine since it's not an outside parameter
	allStatuses.sort(
		(a, b) =>
			new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
	);

	// Determine the reference point for time calculation
	const comparisonDate = referenceDate ?? new Date();

	if (allStatuses.length === 0) {
		return null;
	}

	return (
		<div className="flex flex-col">
			{allStatuses.map((status, index) => {
				const isLatest = index === 0;
				const isFileURI = status.uri?.startsWith("file://");
				const statusTime = new Date(status.created_at);
				// Use formatDistance if referenceDate is provided, otherwise formatDistanceToNow
				const formattedTimestamp = referenceDate
					? formatDistance(statusTime, comparisonDate, { addSuffix: true })
					: formatDistanceToNow(statusTime, { addSuffix: true });

				// Get the associated app for this status
				const currentApp = status.app;
				const agent = agents.find((agent) => agent.id === status.agent_id);

				// Determine if app link should be shown
				const showAppLink =
					isLatest ||
					(index > 0 && status.app_id !== allStatuses[index - 1].app_id);

				return (
					<div
						key={status.id}
						className={`
							flex items-center justify-between px-4 py-3 border-0 [&:not(:last-child)]:border-b border-solid border-border
							${isLatest ? "" : "opacity-50 hover:opacity-100"}
						`}
					>
						<div className="flex flex-col">
							<span className="text-sm font-medium text-content-primary flex items-center gap-2">
								{getStatusIcon(status.state, isLatest)}
								{status.message}
							</span>
							<span className="text-xs text-content-secondary first-letter:uppercase block pl-[26px]">
								{formattedTimestamp}
							</span>
						</div>

						{isLatest && (
							<div className="flex items-center gap-2">
								{currentApp && agent && showAppLink && (
									<AppLink
										app={currentApp}
										agent={agent}
										workspace={workspace}
									/>
								)}

								{status.uri &&
									(isFileURI ? (
										<TooltipProvider>
											<Tooltip>
												<TooltipTrigger>
													<span className="flex items-center gap-1">
														<FileIcon className="size-icon-xs" />
														{formatURI(status.uri)}
													</span>
												</TooltipTrigger>
												<TooltipContent>
													This file is located in your workspace
												</TooltipContent>
											</Tooltip>
										</TooltipProvider>
									) : (
										<Button asChild variant="outline" size="sm">
											<a href={status.uri} target="_blank" rel="noreferrer">
												<ExternalLinkIcon />
												{formatURI(status.uri)}
											</a>
										</Button>
									))}
							</div>
						)}
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
				<ExternalImage src={app.icon} />
				{link.label}
			</a>
		</Button>
	);
};
