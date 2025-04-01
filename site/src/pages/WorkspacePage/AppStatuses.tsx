import { Workspace, WorkspaceAgent } from "api/typesGenerated";
import {
	Box,
	Typography,
	CircularProgress,
	Link,
	Tooltip,
} from "@mui/material";
import { useTheme } from "@mui/material/styles";
import {
	WorkspaceApp,
	WorkspaceAppStatus as APIWorkspaceAppStatus, // Alias to avoid naming conflict
} from "api/typesGenerated";
import { FC } from "react";
import {
	CheckCircle,
	Error,
	Warning,
	OpenInNew,
	InsertDriveFile,
	HelpOutline, // Fallback icon
} from "@mui/icons-material";
import { formatDistanceToNow, formatDistance } from "date-fns";
import { useProxy } from "contexts/ProxyContext";
import { createAppLinkHref } from "utils/apps";
import { Apps as AppsIcon } from "@mui/icons-material";

// --- Copied Helper Functions & Styles ---

const getStatusColor = (theme: any, state: APIWorkspaceAppStatus["state"]) => {
	switch (state) {
		case "complete":
			return theme.palette.success.main;
		case "failure":
			return theme.palette.error.main;
		case "working":
			return theme.palette.primary.main;
		default:
			// Assuming unknown state maps to warning/secondary visually
			return theme.palette.text.secondary;
	}
};

const getStatusIcon = (
	theme: any,
	state: APIWorkspaceAppStatus["state"],
	isLatest: boolean,
) => {
	// Determine color: Use state color if latest, otherwise use disabled text color (grey)
	const color = isLatest
		? getStatusColor(theme, state)
		: theme.palette.text.disabled;
	switch (state) {
		case "complete":
			return <CheckCircle sx={{ color, fontSize: 18 }} />;
		case "failure":
			return <Error sx={{ color, fontSize: 18 }} />;
		case "working":
			return <CircularProgress size={18} sx={{ color }} />;
		default:
			return <Warning sx={{ color, fontSize: 18 }} />;
	}
};

const commonStyles = {
	fontSize: "12px",
	lineHeight: "15px",
	color: "text.disabled",
	display: "inline-flex",
	alignItems: "center",
	gap: 0.5,
	px: 0.75,
	py: 0.25,
	borderRadius: "6px",
	bgcolor: "transparent",
	minWidth: 0,
	maxWidth: "fit-content",
	overflow: "hidden",
	textOverflow: "ellipsis",
	whiteSpace: "nowrap",
	textDecoration: "none",
	transition: "all 0.15s ease-in-out",
	"&:hover": {
		textDecoration: "none",
		bgcolor: "action.hover",
		color: "text.secondary",
	},
	"& .MuiSvgIcon-root": {
		// Consistent icon styling within links
		fontSize: 11,
		opacity: 0.7,
		mt: "-1px", // Slight vertical alignment adjustment
		flexShrink: 0,
	},
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
	const theme = useTheme();
	// Get proxy info for app links
	const { proxy } = useProxy();
	const preferredPathBase = proxy.preferredPathAppURL;
	const appsHost = proxy.preferredWildcardHostname;

	// 1. Flatten all statuses and include the parent app object
	const allStatuses: StatusWithAppInfo[] = apps.flatMap((app) =>
		app.statuses.map((status) => ({
			...status,
			app: app, // Store the parent app object
		})),
	);

	// 2. Sort statuses chronologically (newest first)
	allStatuses.sort(
		(a, b) =>
			new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
	);

	// Determine the reference point for time calculation
	const comparisonDate = referenceDate ?? new Date();

	if (allStatuses.length === 0) {
		return (
			<Typography sx={{ p: 2, color: "text.secondary", textAlign: "center" }}>
				No application statuses reported yet.
			</Typography>
		);
	}

	return (
		<Box sx={{ display: "flex", flexDirection: "column", gap: 2, p: 2 }}>
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
				let appHref: string | undefined;
				const agent = agents.find((agent) => agent.id === status.agent_id);

				if (currentApp && agent) {
					let appSlug = currentApp.slug;
					let appDisplayName = currentApp.display_name;
					if (!appSlug) {
						appSlug = appDisplayName;
					}
					appHref = createAppLinkHref(
						window.location.protocol,
						preferredPathBase,
						appsHost,
						appSlug,
						workspace.owner_name,
						workspace,
						agent,
						currentApp,
					);
				}

				// Determine if app link should be shown
				const showAppLink =
					isLatest ||
					(index > 0 && status.app_id !== allStatuses[index - 1].app_id);

				return (
					<Box
						key={status.id}
						sx={{
							display: "flex",
							alignItems: "flex-start", // Align icon with the first line of text
							gap: 1.5,
							bgcolor: "background.paper",
							borderRadius: 1,
							p: 1.5,
							opacity: isLatest ? 1 : 0.65, // Apply opacity if not the latest
							transition: "opacity 0.15s ease-in-out", // Add smooth transition
							"&:hover": {
								opacity: 1, // Restore opacity on hover for older items
							},
						}}
					>
						{/* Icon Column */}
						<Box
							sx={{
								flexShrink: 0,
								mt: "2px",
								display: "flex",
								alignItems: "center",
							}}
						>
							{getStatusIcon(theme, status.state, isLatest) || (
								<HelpOutline sx={{ fontSize: 18, color: "text.disabled" }} />
							)}
						</Box>

						{/* Content Column */}
						<Box
							sx={{
								display: "flex",
								flexDirection: "column",
								gap: 0.5,
								minWidth: 0,
								flex: 1,
							}}
						>
							{/* Message */}
							<Typography
								sx={{
									fontSize: 14,
									lineHeight: "20px",
									color: "text.primary",
									fontWeight: 500,
								}}
							>
								{status.message}
							</Typography>

							{/* Links Row */}
							<Box
								display="flex"
								flexDirection="column"
								alignItems="flex-start"
								gap={0.5}
								mt={0.5}
								minWidth={0}
							>
								{/* Conditional App Link */}
								{currentApp && appHref && showAppLink && (
									<Tooltip
										title={`Open ${currentApp.display_name}`}
										placement="top"
									>
										<Link
											href={appHref}
											target="_blank"
											rel="noopener"
											sx={{
												...commonStyles,
												position: "relative",
												"& .MuiSvgIcon-root": {
													fontSize: 14,
													opacity: 0.7,
													mr: 0.5,
												},
												"& img": {
													opacity: 0.8,
													marginRight: 0.5,
												},
												"&:hover": {
													...commonStyles["&:hover"],
													color: "text.secondary", // Keep consistent hover color
													"& img": {
														opacity: 1,
													},
													"& .MuiSvgIcon-root": {
														opacity: 1,
													},
												},
											}}
										>
											{currentApp.icon ? (
												<img
													src={currentApp.icon}
													alt={`${currentApp.display_name} icon`}
													width={14}
													height={14}
													style={{ borderRadius: "3px" }}
												/>
											) : (
												<AppsIcon />
											)}
											{/* Keep app name short */}
											<Typography
												component="span"
												variant="caption"
												sx={{
													lineHeight: 1,
													textOverflow: "ellipsis",
													overflow: "hidden",
													whiteSpace: "nowrap",
												}}
											>
												{currentApp.display_name}
											</Typography>
										</Link>
									</Tooltip>
								)}

								{/* Existing URI Link */}
								{status.uri && (
									<Box sx={{ display: "flex", minWidth: 0, width: "100%" }}>
										{" "}
										{isFileURI ? (
											<Tooltip title="This file is located in your workspace">
												<Typography
													sx={{
														...commonStyles,
														"&:hover": {
															bgcolor: "action.hover",
															color: "text.secondary",
														},
													}}
												>
													<InsertDriveFile sx={{ mr: 0.5 }} />
													{formatURI(status.uri)}
												</Typography>
											</Tooltip>
										) : (
											<Link
												href={status.uri}
												target="_blank"
												rel="noopener"
												sx={{
													...commonStyles,
													"&:hover": {
														...commonStyles["&:hover"],
														color: "text.primary", // Keep hover color
													},
												}}
											>
												<OpenInNew sx={{ mr: 0.5 }} />
												<Typography
													sx={{
														bgcolor: "transparent",
														p: 0,
														color: "inherit",
														fontSize: "inherit",
														lineHeight: "inherit",
														overflow: "hidden",
														textOverflow: "ellipsis",
														whiteSpace: "nowrap",
														flexShrink: 1, // Allow text to shrink
													}}
												>
													{formatURI(status.uri)}
												</Typography>
											</Link>
										)}
									</Box>
								)}
							</Box>

							{/* Timestamp */}
							<Typography
								sx={{ fontSize: 12, color: "text.secondary", mt: 0.25 }}
							>
								{formattedTimestamp}
							</Typography>
						</Box>
					</Box>
				);
			})}
		</Box>
	);
};
