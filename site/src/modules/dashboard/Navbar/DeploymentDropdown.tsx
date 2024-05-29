import { css, type Interpolation, type Theme, useTheme } from "@emotion/react";
import Button from "@mui/material/Button";
import MenuItem from "@mui/material/MenuItem";
import { type FC } from "react";
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
        <Button>
          Deployment
          <DropdownArrow
            color={theme.experimental.l2.fill.solid}
            close={false}
          />
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
        <NavLink css={styles.link} to="/deployment/general">
          <MenuItem css={styles.menuItem} onClick={onPopoverClose}>
            Settings
          </MenuItem>
        </NavLink>
      )}
      {canViewAllUsers && (
        <NavLink css={styles.link} to={USERS_LINK}>
          <MenuItem css={styles.menuItem} onClick={onPopoverClose}>
            Users
          </MenuItem>
        </NavLink>
      )}
      {canViewAuditLog && (
        <NavLink css={styles.link} to="/audit">
          <MenuItem css={styles.menuItem} onClick={onPopoverClose}>
            Auditing
          </MenuItem>
        </NavLink>
      )}
      {canViewHealth && (
        <NavLink css={styles.link} to="/health">
          <MenuItem css={styles.menuItem} onClick={onPopoverClose}>
            Healthcheck
          </MenuItem>
        </NavLink>
      )}
    </nav>
  );
};

const styles = {
  link: {
    textDecoration: "none",
    color: "inherit",
  },
  menuItem: (theme) => css`
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
