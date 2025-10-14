import type { Interpolation, Theme } from "@emotion/react";
import Drawer from "@mui/material/Drawer";
import IconButton from "@mui/material/IconButton";
import { visuallyHidden } from "@mui/utils";
import { JobError } from "api/queries/templates";
import type { TemplateVersion } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Loader } from "components/Loader/Loader";
import { TriangleAlertIcon, XIcon } from "lucide-react";
import { AlertVariant } from "modules/provisioners/ProvisionerAlert";
import { ProvisionerStatusAlert } from "modules/provisioners/ProvisionerStatusAlert";
import { useWatchVersionLogs } from "modules/templates/useWatchVersionLogs";
import { WorkspaceBuildLogs } from "modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import type { FC } from "react";
import { navHeight } from "theme/constants";

type BuildLogsDrawerProps = {
	error: unknown;
	open: boolean;
	onClose: () => void;
	templateVersion: TemplateVersion | undefined;
	variablesSectionRef: React.RefObject<HTMLDivElement | null>;
};

export const BuildLogsDrawer: FC<BuildLogsDrawerProps> = ({
	templateVersion,
	error,
	variablesSectionRef,
	...drawerProps
}) => {
	const logs = useWatchVersionLogs(templateVersion);

	const isMissingVariables =
		error instanceof JobError &&
		error.job.error_code === "REQUIRED_TEMPLATE_VARIABLES";

	const matchingProvisioners = templateVersion?.matched_provisioners?.count;
	const availableProvisioners =
		templateVersion?.matched_provisioners?.available;
	const hasLogs = logs && logs.length > 0;

	return (
		<Drawer anchor="right" {...drawerProps}>
			<div css={styles.root}>
				<header css={styles.header}>
					<h3 css={styles.title}>Creating template...</h3>
					<IconButton size="small" onClick={drawerProps.onClose}>
						<XIcon className="size-icon-sm" />
						<span style={visuallyHidden}>Close build logs</span>
					</IconButton>
				</header>

				{isMissingVariables ? (
					<MissingVariablesBanner
						onFillVariables={() => {
							variablesSectionRef.current?.scrollIntoView({
								behavior: "smooth",
							});
							const firstVariableInput =
								variablesSectionRef.current?.querySelector("input");
							firstVariableInput?.focus();
							drawerProps.onClose();
						}}
					/>
				) : (
					<>
						{(matchingProvisioners === 0 || !hasLogs) && (
							<ProvisionerStatusAlert
								matchingProvisioners={matchingProvisioners}
								availableProvisioners={availableProvisioners}
								tags={templateVersion?.job.tags ?? {}}
								variant={AlertVariant.Inline}
							/>
						)}

						{hasLogs ? (
							<section css={styles.logs}>
								<WorkspaceBuildLogs logs={logs} className="border-0" />
							</section>
						) : (
							<Loader />
						)}
					</>
				)}
			</div>
		</Drawer>
	);
};

type MissingVariablesBannerProps = { onFillVariables: () => void };

const MissingVariablesBanner: FC<MissingVariablesBannerProps> = ({
	onFillVariables,
}) => {
	return (
		<div css={bannerStyles.root}>
			<div css={bannerStyles.content}>
				<TriangleAlertIcon className="size-icon-lg" css={bannerStyles.icon} />
				<h4 css={bannerStyles.title}>Missing variables</h4>
				<p css={bannerStyles.description}>
					During the build process, we identified some missing variables. Rest
					assured, we have automatically added them to the form for you.
				</p>
				<Button
					css={bannerStyles.button}
					size="sm"
					variant="outline"
					onClick={onFillVariables}
				>
					Fill variables
				</Button>
			</div>
		</div>
	);
};

const styles = {
	root: {
		width: 800,
		height: "100%",
		display: "flex",
		flexDirection: "column",
	},
	header: (theme) => ({
		height: navHeight,
		padding: "0 24px",
		display: "flex",
		alignItems: "center",
		justifyContent: "space-between",
		borderBottom: `1px solid ${theme.palette.divider}`,
	}),
	title: {
		margin: 0,
		fontWeight: 500,
		fontSize: 16,
	},
	logs: (theme) => ({
		flex: 1,
		overflow: "auto",
		backgroundColor: theme.palette.background.default,
	}),
} satisfies Record<string, Interpolation<Theme>>;

const bannerStyles = {
	root: {
		display: "flex",
		alignItems: "center",
		justifyContent: "center",
		padding: 40,
	},
	content: {
		display: "flex",
		flexDirection: "column",
		alignItems: "center",
		textAlign: "center",
		maxWidth: 360,
	},
	icon: (theme) => ({
		color: theme.roles.warning.fill.outline,
	}),
	title: {
		fontWeight: 500,
		lineHeight: "1",
		margin: 0,
		marginTop: 16,
	},
	description: (theme) => ({
		color: theme.palette.text.secondary,
		fontSize: 14,
		margin: 0,
		marginTop: 8,
		lineHeight: "1.5",
	}),
	button: {
		marginTop: 16,
	},
} satisfies Record<string, Interpolation<Theme>>;
