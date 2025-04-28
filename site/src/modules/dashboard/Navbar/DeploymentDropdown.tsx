import { type Interpolation, type Theme, css, useTheme } from "@emotion/react";
import MenuItem from "@mui/material/MenuItem";
import { Button } from "components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
	usePopover,
} from "components/deprecated/Popover/Popover";
import { ChevronDownIcon } from "lucide-react";
import { linkToAuditing } from "modules/navigation";
import type { FC } from "react";
import { NavLink } from "react-router-dom";

interface DeploymentDropdownProps {
	canViewDeployment: boolean;
	canViewOrganizations: boolean;
	canViewAuditLog: boolean;
	canViewHealth: boolean;
}

export const DeploymentDropdown: FC<DeploymentDropdownProps> = ({
	canViewDeployment,
	canViewOrganizations,
	canViewAuditLog,
	canViewHealth,
}) => {
	const theme = useTheme();

	if (
		!canViewAuditLog &&
		!canViewOrganizations &&
		!canViewDeployment &&
		!canViewHealth
	) {
		return null;
	}

	return (
		<Popover>
			<PopoverTrigger>
				<Button variant="outline" size="lg">
					Admin settings
					<ChevronDownIcon className="text-content-primary !size-icon-xs" />
				</Button>
			</PopoverTrigger>

			<PopoverContent
				horizontal="right"
				css={{
					".MuiPaper-root": {
						minWidth: "auto",
						width: 180,
						boxShadow: theme.shadows[6],
					},
				}}
			>
				<DeploymentDropdownContent
					canViewDeployment={canViewDeployment}
					canViewOrganizations={canViewOrganizations}
					canViewAuditLog={canViewAuditLog}
					canViewHealth={canViewHealth}
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
}) => {
	const popover = usePopover();

	const onPopoverClose = () => popover.setOpen(false);

	return (
		<nav>
			{canViewDeployment && (
				<MenuItem
					component={NavLink}
					to="/deployment"
					css={styles.menuItem}
					onClick={onPopoverClose}
				>
					Deployment
				</MenuItem>
			)}
			{canViewOrganizations && (
				<MenuItem
					component={NavLink}
					to="/organizations"
					css={styles.menuItem}
					onClick={onPopoverClose}
				>
					Organizations
				</MenuItem>
			)}
			{canViewAuditLog && (
				<MenuItem
					component={NavLink}
					to={linkToAuditing}
					css={styles.menuItem}
					onClick={onPopoverClose}
				>
					Audit Logs
				</MenuItem>
			)}
			{canViewHealth && (
				<MenuItem
					component={NavLink}
					to="/health"
					css={styles.menuItem}
					onClick={onPopoverClose}
				>
					Healthcheck
				</MenuItem>
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
