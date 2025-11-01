import type { Interpolation, Theme } from "@emotion/react";
import type { WorkspaceAgent } from "api/typesGenerated";
import { TerminalIcon } from "components/Icons/TerminalIcon";
import { VSCodeIcon } from "components/Icons/VSCodeIcon";
import type { FC } from "react";
import { cn } from "utils/cn";
import { DisplayAppNameMap } from "./AppLink/AppLink";
import { AppPreview } from "./AppLink/AppPreview";
import { BaseIcon } from "./AppLink/BaseIcon";

interface AgentRowPreviewStyles {
	// Helpful when there are more than one row so the values are aligned
	// When it is only one row, it is better to have than "flex" and not hard aligned
	alignValues?: boolean;
}
interface AgentRowPreviewProps extends AgentRowPreviewStyles {
	agent: WorkspaceAgent;
}

export const AgentRowPreview: FC<AgentRowPreviewProps> = ({
	agent,
	alignValues,
}) => {
	return (
		<div
			key={agent.id}
			className="flex flex-row items-center justify-between"
			css={styles.agentRow}
		>
			<div className="flex flex-row items-baseline">
				<div css={styles.agentStatusWrapper}>
					<div css={styles.agentStatusPreview} />
				</div>
				<div
					className="flex flex-row items-baseline gap-8 md:flex-wrap md:gap-4"
					css={styles.agentData}
				>
					<div
						className={cn(
							"flex flex-row items-baseline gap-2 flex-shrink-0 md:flex-col md:items-start md:gap-2 md:w-fit",
						)}
						css={(theme) => ({
							[theme.breakpoints.up("sm")]: {
								minWidth: alignValues ? 240 : undefined,
							},
						})}
					>
						<span>Agent:</span>
						<span css={styles.agentDataValue}>{agent.name}</span>
					</div>

					<div
						className={cn(
							"flex flex-row items-baseline gap-2 flex-shrink-0 md:flex-col md:items-start md:gap-2 md:w-fit",
						)}
						css={(theme) => ({
							[theme.breakpoints.up("sm")]: {
								minWidth: alignValues ? 100 : undefined,
							},
						})}
					>
						<span>OS:</span>
						<span css={[styles.agentDataValue, styles.agentOS]}>
							{agent.operating_system}
						</span>
					</div>

					<div className="flex flex-row items-center gap-2 md:flex-col md:items-start md:gap-2 md:w-fit">
						<span>Apps:</span>
						<div className="flex flex-row items-center gap-1 flex-wrap">
							{/* We display all modules returned in agent.apps */}
							{agent.apps.map((app) => (
								<AppPreview key={app.slug}>
									<BaseIcon app={app} />
									{app.display_name}
								</AppPreview>
							))}
							{/* Additionally, we display any apps that are visible, e.g.
              apps that are included in agent.display_apps */}
							{agent.display_apps.includes("web_terminal") && (
								<AppPreview>
									<TerminalIcon sx={{ width: 12, height: 12 }} />
									{DisplayAppNameMap.web_terminal}
								</AppPreview>
							)}
							{agent.display_apps.includes("ssh_helper") && (
								<AppPreview>{DisplayAppNameMap.ssh_helper}</AppPreview>
							)}
							{agent.display_apps.includes("port_forwarding_helper") && (
								<AppPreview>
									{DisplayAppNameMap.port_forwarding_helper}
								</AppPreview>
							)}
							{/* VSCode display apps (vscode, vscode_insiders) get special presentation */}
							{agent.display_apps.includes("vscode") ? (
								<AppPreview>
									<VSCodeIcon sx={{ width: 12, height: 12 }} />
									{DisplayAppNameMap.vscode}
								</AppPreview>
							) : (
								agent.display_apps.includes("vscode_insiders") && (
									<AppPreview>
										<VSCodeIcon sx={{ width: 12, height: 12 }} />
										{DisplayAppNameMap.vscode_insiders}
									</AppPreview>
								)
							)}
							{agent.apps.length === 0 && agent.display_apps.length === 0 && (
								<span css={styles.agentDataValue}>None</span>
							)}
						</div>
					</div>
				</div>
			</div>
		</div>
	);
};

const styles = {
	agentRow: (theme) => ({
		padding: "16px 32px",
		backgroundColor: theme.palette.background.paper,
		fontSize: 16,
		position: "relative",

		"&:not(:last-child)": {
			paddingBottom: 0,
		},

		"&:after": {
			content: "''",
			height: "100%",
			width: 2,
			backgroundColor: theme.palette.divider,
			position: "absolute",
			top: 0,
			left: 43,
		},
	}),

	agentStatusWrapper: {
		width: 24,
		display: "flex",
		justifyContent: "center",
		flexShrink: 0,
	},

	agentStatusPreview: (theme) => ({
		width: 10,
		height: 10,
		border: `2px solid ${theme.palette.text.secondary}`,
		borderRadius: "100%",
		position: "relative",
		zIndex: 1,
		background: theme.palette.background.paper,
	}),

	agentName: {
		fontWeight: 600,
	},

	agentOS: {
		textTransform: "capitalize",
		fontSize: 14,
	},

	agentData: (theme) => ({
		fontSize: 14,
		color: theme.palette.text.secondary,

		[theme.breakpoints.down("md")]: {
			gap: 16,
			flexWrap: "wrap",
		},
	}),

	agentDataValue: (theme) => ({
		color: theme.palette.text.primary,
	}),
} satisfies Record<string, Interpolation<Theme>>;
