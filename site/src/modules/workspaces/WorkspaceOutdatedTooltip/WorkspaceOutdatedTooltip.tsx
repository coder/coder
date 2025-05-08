import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import Link from "@mui/material/Link";
import Skeleton from "@mui/material/Skeleton";
import { getErrorDetail, getErrorMessage } from "api/errors";
import { templateVersion } from "api/queries/templates";
import type { Workspace } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import {
	HelpTooltip,
	HelpTooltipAction,
	HelpTooltipContent,
	HelpTooltipLinksGroup,
	HelpTooltipText,
	HelpTooltipTitle,
	HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import { usePopover } from "components/deprecated/Popover/Popover";
import { InfoIcon, RefreshIcon } from "lucide-react";
import { linkToTemplate, useLinks } from "modules/navigation";
import type { FC } from "react";
import { useQuery } from "react-query";
import {
	WorkspaceUpdateDialogs,
	useWorkspaceUpdate,
} from "../WorkspaceUpdateDialogs";

interface TooltipProps {
	workspace: Workspace;
}

export const WorkspaceOutdatedTooltip: FC<TooltipProps> = (props) => {
	return (
		<HelpTooltip>
			<HelpTooltipTrigger size="small" hoverEffect={false}>
				<InfoIcon css={styles.icon} />
				<span className="sr-only">Outdated info</span>
			</HelpTooltipTrigger>
			<WorkspaceOutdatedTooltipContent {...props} />
		</HelpTooltip>
	);
};

const WorkspaceOutdatedTooltipContent: FC<TooltipProps> = ({ workspace }) => {
	const getLink = useLinks();
	const theme = useTheme();
	const popover = usePopover();
	const { data: activeVersion } = useQuery({
		...templateVersion(workspace.template_active_version_id),
		enabled: popover.open,
	});
	const updateWorkspace = useWorkspaceUpdate({
		workspace,
		latestVersion: activeVersion,
		onError: (error) => {
			displayError(
				getErrorMessage(error, "Error updating workspace"),
				getErrorDetail(error),
			);
		},
	});

	const versionLink = `${getLink(
		linkToTemplate(workspace.organization_name, workspace.template_name),
	)}`;

	return (
		<>
			<HelpTooltipContent disablePortal={false}>
				<HelpTooltipTitle>Outdated</HelpTooltipTitle>
				<HelpTooltipText>
					This workspace version is outdated and a newer version is available.
				</HelpTooltipText>

				<div css={styles.container}>
					<div css={{ lineHeight: "1.6" }}>
						<div css={styles.bold}>New version</div>
						<div>
							{activeVersion ? (
								<Link
									href={`${versionLink}/versions/${activeVersion.name}`}
									target="_blank"
									css={{ color: theme.palette.primary.light }}
								>
									{activeVersion.name}
								</Link>
							) : (
								<Skeleton variant="text" height={20} width={100} />
							)}
						</div>
					</div>

					<div css={{ lineHeight: "1.6" }}>
						<div css={styles.bold}>Message</div>
						<div>
							{activeVersion ? (
								activeVersion.message || "No message"
							) : (
								<Skeleton variant="text" height={20} width={150} />
							)}
						</div>
					</div>
				</div>

				<HelpTooltipLinksGroup>
					<HelpTooltipAction
						icon={RefreshIcon}
						onClick={updateWorkspace.update}
					>
						Update
					</HelpTooltipAction>
				</HelpTooltipLinksGroup>
			</HelpTooltipContent>
			<WorkspaceUpdateDialogs {...updateWorkspace.dialogs} />
		</>
	);
};

const styles = {
	icon: (theme) => ({
		color: theme.roles.notice.outline,
	}),

	container: {
		display: "flex",
		flexDirection: "column",
		gap: 8,
		paddingTop: 8,
		paddingBottom: 8,
		fontSize: 13,
	},

	bold: (theme) => ({
		color: theme.palette.text.primary,
		fontWeight: 600,
	}),
} satisfies Record<string, Interpolation<Theme>>;
