import { css, type Interpolation, type Theme, useTheme } from "@emotion/react";
import Button from "@mui/material/Button";
import MenuItem from "@mui/material/MenuItem";
import type { FC } from "react";
import { NavLink } from "react-router-dom";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
  usePopover,
} from "components/Popover/Popover";
import { USERS_LINK } from "modules/navigation";

interface DeploymentDropdownProps {
  canViewAuditLog: boolean;
  canViewDeployment: boolean;
  canViewAllUsers: boolean;
  canViewHealth: boolean;
}

export const DeploymentDropdown: FC<DeploymentDropdownProps> = ({
  canViewAuditLog,
  canViewDeployment,
  canViewAllUsers,
  canViewHealth,
}) => {
  const theme = useTheme();

  if (
    !canViewAuditLog &&
    !canViewDeployment &&
    !canViewAllUsers &&
    !canViewHealth
  ) {
    return null;
  }

  return (
    <Popover>
      <PopoverTrigger>
        <Button
          size="small"
          endIcon={
            <DropdownArrow
              color={theme.experimental.l2.fill.solid}
              close={false}
              margin={false}
            />
          }
        >
          Deployment
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
          canViewAuditLog={canViewAuditLog}
          canViewDeployment={canViewDeployment}
          canViewAllUsers={canViewAllUsers}
          canViewHealth={canViewHealth}
        />
      </PopoverContent>
    </Popover>
  );
};

const DeploymentDropdownContent: FC<DeploymentDropdownProps> = ({
  canViewAuditLog,
  canViewDeployment,
  canViewAllUsers,
  canViewHealth,
}) => {
  const popover = usePopover();

  const onPopoverClose = () => popover.setIsOpen(false);

  return (
    <nav>
      {canViewDeployment && (
        <MenuItem
          component={NavLink}
          to="/deployment/general"
          css={styles.menuItem}
          onClick={onPopoverClose}
        >
          Settings
        </MenuItem>
      )}
      {canViewAllUsers && (
        <MenuItem
          component={NavLink}
          to={USERS_LINK}
          css={styles.menuItem}
          onClick={onPopoverClose}
        >
          Users
        </MenuItem>
      )}
      {canViewAuditLog && (
        <MenuItem
          component={NavLink}
          to="/audit"
          css={styles.menuItem}
          onClick={onPopoverClose}
        >
          Auditing
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
    gap: 20px;
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
