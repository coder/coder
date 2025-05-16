import type { Theme } from "@emotion/react";
import { useTheme } from "@emotion/react";
import AppsIcon from "@mui/icons-material/Apps";
import CheckCircle from "@mui/icons-material/CheckCircle";
import ErrorIcon from "@mui/icons-material/Error";
import InsertDriveFile from "@mui/icons-material/InsertDriveFile";
import Warning from "@mui/icons-material/Warning";
import CircularProgress from "@mui/material/CircularProgress";
import type {
	WorkspaceAppStatus as APIWorkspaceAppStatus,
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "api/typesGenerated";
import { ExternalLinkIcon } from "lucide-react";
import { useAppLink } from "modules/apps/useAppLink";
import type { FC } from "react";

const formatURI = (uri: string) => {
	try {
		const url = new URL(uri);
		return url.hostname + url.pathname;
	} catch {
		return uri;
	}
};

const getStatusColor = (
	theme: Theme,
	state: APIWorkspaceAppStatus["state"],
) => {
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

const getStatusIcon = (theme: Theme, state: APIWorkspaceAppStatus["state"]) => {
	const color = getStatusColor(theme, state);
	switch (state) {
		case "complete":
			return <CheckCircle sx={{ color, fontSize: 16 }} />;
		case "failure":
			return <ErrorIcon sx={{ color, fontSize: 16 }} />;
		case "working":
			return <CircularProgress size={16} sx={{ color }} />;
		default:
			return <Warning sx={{ color, fontSize: 16 }} />;
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
	const commonStyles = useCommonStyles();

	if (!status) {
		return (
			<div
				css={{
					display: "flex",
					alignItems: "center",
					gap: 12,
					minWidth: 0,
					paddingRight: 16,
				}}
			>
				<div
					css={{
						fontSize: "14px",
						color: theme.palette.text.disabled,
						flexShrink: 1,
						minWidth: 0,
					}}
				>
					―
				</div>
			</div>
		);
	}
	const isFileURI = status.uri?.startsWith("file://");

	return (
		<div
			css={{
				display: "flex",
				alignItems: "flex-start",
				gap: 8,
				minWidth: 0,
				paddingRight: 16,
			}}
		>
			<div
				css={{
					display: "flex",
					alignItems: "center",
					flexShrink: 0,
					marginTop: 2,
				}}
			>
				{getStatusIcon(theme, status.state)}
			</div>
			<div
				css={{
					display: "flex",
					flexDirection: "column",
					gap: 6,
					minWidth: 0,
					flex: 1,
				}}
			>
				<div
					css={{
						fontSize: "14px",
						lineHeight: "20px",
						color: "text.primary",
						margin: 0,
						display: "-webkit-box",
						WebkitLineClamp: 2,
						WebkitBoxOrient: "vertical",
						overflow: "hidden",
						textOverflow: "ellipsis",
						maxWidth: "100%",
					}}
				>
					{status.message}
				</div>
				<div
					css={{
						display: "flex",
						alignItems: "center",
					}}
				>
					{app && agent && (
						<AppLink app={app} workspace={workspace} agent={agent} />
					)}
					{status.uri && (
						<div
							css={{
								display: "flex",
								minWidth: 0,
							}}
						>
							{isFileURI ? (
								<div
									css={{
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
									<span>{formatURI(status.uri)}</span>
								</div>
							) : (
								<a
									href={status.uri}
									target="_blank"
									rel="noopener noreferrer"
									css={{
										...commonStyles,
										color: theme.palette.text.secondary,
										"&:hover": {
											...commonStyles["&:hover"],
											color: theme.palette.text.primary,
										},
									}}
								>
									<ExternalLinkIcon
										className="size-icon-xs"
										css={{
											opacity: 0.7,
											flexShrink: 0,
											marginRight: 2,
										}}
									/>
									<span
										css={{
											backgroundColor: "transparent",
											padding: 0,
											color: "inherit",
											fontSize: "inherit",
											lineHeight: "inherit",
											overflow: "hidden",
											textOverflow: "ellipsis",
											whiteSpace: "nowrap",
										}}
									>
										{formatURI(status.uri)}
									</span>
								</a>
							)}
						</div>
					)}
				</div>
			</div>
		</div>
	);
};

type AppLinkProps = {
	app: WorkspaceApp;
	workspace: Workspace;
	agent: WorkspaceAgent;
};

const AppLink: FC<AppLinkProps> = ({ app, workspace, agent }) => {
	const theme = useTheme();
	const commonStyles = useCommonStyles();
	const link = useAppLink(app, { agent, workspace });

	return (
		<a
			href={link.href}
			onClick={link.onClick}
			target="_blank"
			rel="noopener noreferrer"
			css={{
				...commonStyles,
				marginRight: 8,
				position: "relative",
				color: theme.palette.text.secondary,
				"&:hover": {
					...commonStyles["&:hover"],
					color: theme.palette.text.primary,
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
					css={{
						borderRadius: "3px",
						opacity: 0.8,
						marginRight: 4,
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
			<span>{app.display_name}</span>
		</a>
	);
};

const useCommonStyles = () => {
	const theme = useTheme();

	return {
		fontSize: "12px",
		lineHeight: "15px",
		color: theme.palette.text.disabled,
		display: "inline-flex",
		alignItems: "center",
		gap: 4,
		padding: "2px 6px",
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
			backgroundColor: theme.palette.action.hover,
			color: theme.palette.text.secondary,
		},
	};
};
