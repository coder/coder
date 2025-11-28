import { css, type Interpolation, type Theme } from "@emotion/react";
import MenuItem from "@mui/material/MenuItem";
import { Button } from "components/Button/Button";
import {
	Popover,
	PopoverClose,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { ChevronDownIcon } from "lucide-react";
import { linkToAuditing } from "modules/navigation";
import type { FC } from "react";
import { NavLink } from "react-router";

interface DeploymentDropdownProps {
	canViewDeployment: boolean;
	canViewOrganizations: boolean;
	canViewAuditLog: boolean;
	canViewConnectionLog: boolean;
	canViewHealth: boolean;
	canViewAIBridge: boolean;
}

export const DeploymentDropdown: FC<DeploymentDropdownProps> = ({
	canViewDeployment,
	canViewOrganizations,
	canViewAuditLog,
	canViewConnectionLog,
	canViewHealth,
	canViewAIBridge,
}) => {
	if (
		!canViewAuditLog &&
		!canViewConnectionLog &&
		!canViewOrganizations &&
		!canViewDeployment &&
		!canViewHealth &&
		!canViewAIBridge
	) {
		return null;
	}

	return (
		<Popover>
			<PopoverTrigger asChild>
				<Button variant="outline" size="lg">
					Admin settings
					<ChevronDownIcon className="text-content-primary !size-icon-xs" />
				</Button>
			</PopoverTrigger>

			<PopoverContent
				align="end"
				className="bg-surface-secondary border-surface-quaternary w-[180px] min-w-auto"
			>
				<DeploymentDropdownContent
					canViewDeployment={canViewDeployment}
					canViewOrganizations={canViewOrganizations}
					canViewAuditLog={canViewAuditLog}
					canViewConnectionLog={canViewConnectionLog}
					canViewHealth={canViewHealth}
					canViewAIBridge={canViewAIBridge}
				/>
			</PopoverContent>
		</Popover>
	);
};

const DeploymentDropdownContent: FC<DeploymentDropdownProps> = ({
	canViewDeployment,
	canViewOrganizations,
	canViewAuditLog,
	canViewHealth,
	canViewConnectionLog,
	canViewAIBridge,
}) => {
	return (
		<nav>
			{canViewDeployment && (
				<PopoverClose asChild>
					<MenuItem component={NavLink} to="/deployment" css={styles.menuItem}>
						Deployment
					</MenuItem>
				</PopoverClose>
			)}
			{canViewOrganizations && (
				<PopoverClose asChild>
					<MenuItem
						component={NavLink}
						to="/organizations"
						css={styles.menuItem}
					>
						Organizations
					</MenuItem>
				</PopoverClose>
			)}
			{canViewAuditLog && (
				<PopoverClose asChild>
					<MenuItem
						component={NavLink}
						to={linkToAuditing}
						css={styles.menuItem}
					>
						Audit Logs
					</MenuItem>
				</PopoverClose>
			)}
			{canViewConnectionLog && (
				<PopoverClose asChild>
					<MenuItem
						component={NavLink}
						to="/connectionlog"
						css={styles.menuItem}
					>
						Connection Logs
					</MenuItem>
				</PopoverClose>
			)}
			{canViewAIBridge && (
				<PopoverClose asChild>
					<MenuItem component={NavLink} to="/aibridge" css={styles.menuItem}>
						AI Bridge
					</MenuItem>
				</PopoverClose>
			)}
			{canViewHealth && (
				<PopoverClose asChild>
					<MenuItem component={NavLink} to="/health" css={styles.menuItem}>
						Healthcheck
					</MenuItem>
				</PopoverClose>
			)}
		</nav>
	);
};

const styles = {
	menuItem: (theme) => css`
		text-decoration: none;
		color: inherit;
		gap: 8px;
		padding: 8px 20px;
		font-size: 14px;

		&:hover {
			background-color: ${theme.palette.action.hover};
			transition: background-color 0.3s ease;
		}
	`,
	menuItemIcon: (theme) => ({
		color: theme.palette.text.secondary,
		width: 20,
		height: 20,
	}),
} satisfies Record<string, Interpolation<Theme>>;
