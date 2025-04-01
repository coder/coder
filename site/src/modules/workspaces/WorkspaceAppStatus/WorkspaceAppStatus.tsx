import { useTheme } from "@emotion/react";
import type { Theme } from "@emotion/react";
import type {
	WorkspaceAppStatus as APIWorkspaceAppStatus,
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "api/typesGenerated";
import CheckCircle from "@mui/icons-material/CheckCircle";
import ErrorIcon from "@mui/icons-material/Error";
import Warning from "@mui/icons-material/Warning";
import OpenInNew from "@mui/icons-material/OpenInNew";
import InsertDriveFile from "@mui/icons-material/InsertDriveFile";
import AppsIcon from "@mui/icons-material/Apps";
import { createAppLinkHref } from "utils/apps";
import { useProxy } from "contexts/ProxyContext";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";

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
	try {
		const url = new URL(uri);
		return url.hostname + url.pathname;
	} catch {
		return uri;
	}
};

const getStatusIcon = (theme: Theme, state: APIWorkspaceAppStatus["state"]) => {
	switch (state) {
		case "running":
			return (
				<CheckCircle
					sx={{
						fontSize: 16,
						color: theme.palette.success.main,
					}}
				/>
			);
		case "error":
			return (
				<ErrorIcon
					sx={{
						fontSize: 16,
						color: theme.palette.error.main,
					}}
				/>
			);
		case "starting":
			return (
				<Warning
					sx={{
						fontSize: 16,
						color: theme.palette.warning.main,
					}}
				/>
			);
		default:
			return null;
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
				<Typography
					sx={{
						fontSize: "14px",
						color: "text.disabled",
						flexShrink: 1,
						minWidth: 0,
					}}
				>
					―
				</Typography>
			</Box>
		);
	}
	const isFileURI = status.uri?.startsWith("file://");

	let appHref: string | undefined;
	if (app && agent) {
		const appSlug = app.slug || app.display_name;
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
					mt: 0.25,
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
						fontSize: "14px",
						lineHeight: "20px",
						color: "text.primary",
						m: 0,
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
				<Box
					sx={{
						display: "flex",
						alignItems: "center",
					}}
				>
					{app && appHref && (
						<Box
							component="a"
							href={appHref}
							target="_blank"
							rel="noopener"
							sx={{
								...commonStyles,
								mr: 1,
								position: "relative",
								"&:hover": {
									...commonStyles["&:hover"],
									"& img": {
										opacity: 1,
									},
								},
							}}
						>
							{app.icon ? (
								<Box
									component="img"
									src={app.icon}
									alt={`${app.display_name} icon`}
									width={14}
									height={14}
									sx={{
										borderRadius: "3px",
										opacity: 0.8,
										mr: 0.5,
									}}
								/>
							) : (
								<AppsIcon
									sx={{
										fontSize: 14,
										opacity: 0.7,
									}}
								/>
							)}
							<Typography component="span">{app.display_name}</Typography>
						</Box>
					)}
					{status.uri && (
						<Box
							sx={{
								display: "flex",
								minWidth: 0,
							}}
						>
							{isFileURI ? (
								<Box
									sx={{
										...commonStyles,
									}}
								>
									<InsertDriveFile
										sx={{
											fontSize: "11px",
											opacity: 0.5,
											mr: 0.25,
										}}
									/>
									<Typography component="span">
										{formatURI(status.uri)}
									</Typography>
								</Box>
							) : (
								<Box
									component="a"
									href={status.uri}
									target="_blank"
									rel="noopener"
									sx={{
										...commonStyles,
										"&:hover": {
											...commonStyles["&:hover"],
											color: "text.primary",
										},
									}}
								>
									<OpenInNew
										sx={{
											fontSize: 11,
											opacity: 0.7,
											mt: -0.125,
											flexShrink: 0,
											mr: 0.5,
										}}
									/>
									<Typography
										component="span"
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
								</Box>
							)}
						</Box>
					)}
				</Box>
			</Box>
		</Box>
	);
};
