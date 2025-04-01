import {
	Box,
	Typography,
	CircularProgress,
	Link,
	Tooltip,
} from "@mui/material";
import { useTheme } from "@mui/material/styles";
import {
	WorkspaceAppStatus as APIWorkspaceAppStatus,
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "api/typesGenerated";
import {
	CheckCircle,
	Error,
	Warning,
	OpenInNew,
	InsertDriveFile,
	Apps as AppsIcon,
} from "@mui/icons-material";
import { createAppLinkHref } from "utils/apps";
import { useProxy } from "contexts/ProxyContext";

const getStatusColor = (theme: any, state: APIWorkspaceAppStatus["state"]) => {
	switch (state) {
		case "complete":
			return theme.palette.success.main;
		case "failure":
			return theme.palette.error.main;
		case "working":
			return theme.palette.primary.main;
		default:
			return theme.palette.text.secondary;
	}
};

const getStatusIcon = (theme: any, state: APIWorkspaceAppStatus["state"]) => {
	const color = getStatusColor(theme, state);
	switch (state) {
		case "complete":
			return <CheckCircle sx={{ color }} />;
		case "failure":
			return <Error sx={{ color }} />;
		case "working":
			return <CircularProgress size={16} sx={{ color }} />;
		default:
			return <Warning sx={{ color }} />;
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
};

const formatURI = (uri: string) => {
	if (uri.startsWith("file://")) {
		const path = uri.slice(7);
		if (path.length > 40) {
			const start = path.slice(0, 20);
			const end = path.slice(-20);
			return `${start}...${end}`;
		}
		return path;
	}

	try {
		const url = new URL(uri);
		const fullUrl = url.toString();
		if (fullUrl.length > 50) {
			const start = fullUrl.slice(0, 25);
			const end = fullUrl.slice(-25);
			return `${start}...${end}`;
		}
		return fullUrl;
	} catch {
		if (uri.length > 40) {
			const start = uri.slice(0, 20);
			const end = uri.slice(-20);
			return `${start}...${end}`;
		}
		return uri;
	}
};

export const WorkspaceAppStatus = ({
	workspace,
	status,
	agent,
	app,
}: {
	workspace: Workspace;
	status?: APIWorkspaceAppStatus | null;
	app?: WorkspaceApp;
	agent?: WorkspaceAgent;
}) => {
	const theme = useTheme();
	const { proxy } = useProxy();
	const preferredPathBase = proxy.preferredPathAppURL;
	const appsHost = proxy.preferredWildcardHostname;

	if (!status) {
		return (
			<Box
				sx={{
					display: "flex",
					alignItems: "center",
					gap: 1.5,
					minWidth: 0,
					pr: 2,
				}}
			>
				<Tooltip title="No apps have reported a status">
					<Typography
						sx={{
							fontSize: 14,
							color: "text.disabled",
							flexShrink: 1,
							minWidth: 0,
						}}
					>
						―
					</Typography>
				</Tooltip>
			</Box>
		);
	}
	const isFileURI = status.uri?.startsWith("file://");

	let appHref: string | undefined;
	if (app && agent) {
		let appSlug = app.slug;
		let appDisplayName = app.display_name;
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
			app,
		);
	}

	return (
		<Box
			sx={{
				display: "flex",
				alignItems: "flex-start",
				gap: 1.5,
				minWidth: 0,
				pr: 2,
			}}
		>
			<Box
				sx={{
					display: "flex",
					alignItems: "center",
					flexShrink: 0,
					marginTop: "2px",
					"& svg": {
						fontSize: 16,
					},
				}}
			>
				{getStatusIcon(theme, status.state)}
			</Box>
			<Box
				sx={{
					display: "flex",
					flexDirection: "column",
					gap: 0.75,
					minWidth: 0,
					flex: 1,
				}}
			>
				<Typography
					sx={{
						fontSize: 14,
						lineHeight: "20px",
						color: "text.primary",
						display: "-webkit-box",
						WebkitLineClamp: 2,
						WebkitBoxOrient: "vertical",
						overflow: "hidden",
						textOverflow: "ellipsis",
						maxWidth: "100%",
					}}
				>
					{status.message}
				</Typography>
				<Box display="flex" alignItems="center">
					{app && appHref && (
						<Tooltip title={`Open ${app.display_name}`} placement="top">
							<Link
								href={appHref}
								target="_blank"
								rel="noopener"
								sx={{
									...commonStyles,
									marginRight: 1,
									position: "relative",
									"& .MuiSvgIcon-root": {
										fontSize: 14,
										opacity: 0.7,
									},
									"& img": {
										opacity: 0.8,
										marginRight: 0.5,
									},
									"&:hover": {
										...commonStyles["&:hover"],
										"& img": {
											opacity: 1,
										},
									},
								}}
							>
								{app.icon ? (
									<img
										src={app.icon}
										alt={`${app.display_name} icon`}
										width={14}
										height={14}
										style={{ borderRadius: "3px" }}
									/>
								) : (
									<AppsIcon />
								)}

								<span>{app.display_name}</span>
							</Link>
						</Tooltip>
					)}
					{status.uri && (
						<Box sx={{ display: "flex", minWidth: 0 }}>
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
										<InsertDriveFile
											sx={{ fontSize: "11px", opacity: 0.5, mr: 0.25 }}
										/>
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
										"& .MuiSvgIcon-root": {
											fontSize: 11,
											opacity: 0.7,
											mt: "-1px",
											flexShrink: 0,
											marginRight: 0.5,
										},
										"&:hover": {
											...commonStyles["&:hover"],
											color: "text.primary",
										},
									}}
								>
									<OpenInNew />
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
										}}
									>
										{formatURI(status.uri)}
									</Typography>
								</Link>
							)}
						</Box>
					)}
				</Box>
			</Box>
		</Box>
	);
};
