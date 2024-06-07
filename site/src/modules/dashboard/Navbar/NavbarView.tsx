import { css, type Interpolation, type Theme, useTheme } from "@emotion/react";
import MenuIcon from "@mui/icons-material/Menu";
import Drawer from "@mui/material/Drawer";
import IconButton from "@mui/material/IconButton";
import { type FC, useState } from "react";
import { NavLink, useLocation } from "react-router-dom";
import type * as TypesGen from "api/typesGenerated";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CoderIcon } from "components/Icons/CoderIcon";
import type { ProxyContextValue } from "contexts/ProxyContext";
import { navHeight } from "theme/constants";
import { DeploymentDropdown } from "./DeploymentDropdown";
import { ProxyMenu } from "./ProxyMenu";
import { UserDropdown } from "./UserDropdown/UserDropdown";

export interface NavbarViewProps {
  logo_url?: string;
  user?: TypesGen.User;
  buildInfo?: TypesGen.BuildInfoResponse;
  supportLinks?: readonly TypesGen.LinkConfig[];
  onSignOut: () => void;
  canViewDeployment: boolean;
  canViewOrganizations: boolean;
  canViewAllUsers: boolean;
  canViewAuditLog: boolean;
  canViewHealth: boolean;
  proxyContextValue?: ProxyContextValue;
}

export const Language = {
  workspaces: "Workspaces",
  templates: "Templates",
  users: "Users",
  audit: "Auditing",
  deployment: "Settings",
};

interface NavItemsProps {
  className?: string;
}

const NavItems: FC<NavItemsProps> = ({ className }) => {
  const location = useLocation();
  const theme = useTheme();

  return (
    <nav className={className}>
      <NavLink
        css={[
          styles.link,
          location.pathname.startsWith("/@") && {
            color: theme.palette.text.primary,
            fontWeight: 500,
          },
        ]}
        to="/workspaces"
      >
        {Language.workspaces}
      </NavLink>
      <NavLink css={styles.link} to="/templates">
        {Language.templates}
      </NavLink>
    </nav>
  );
};

export const NavbarView: FC<NavbarViewProps> = ({
  user,
  logo_url,
  buildInfo,
  supportLinks,
  onSignOut,
  canViewDeployment,
  canViewOrganizations,
  canViewAllUsers,
  canViewHealth,
  canViewAuditLog,
  proxyContextValue,
}) => {
  const theme = useTheme();
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);

  return (
    <nav
      css={{
        height: navHeight,
        backgroundColor: theme.palette.background.paper,
        borderBottom: `1px solid ${theme.palette.divider}`,
      }}
    >
      <div css={styles.wrapper}>
        <IconButton
          aria-label="Open menu"
          css={styles.mobileMenuButton}
          onClick={() => {
            setIsDrawerOpen(true);
          }}
          size="large"
        >
          <MenuIcon />
        </IconButton>

        <Drawer
          anchor="left"
          open={isDrawerOpen}
          onClose={() => setIsDrawerOpen(false)}
        >
          <div css={{ width: 250 }}>
            <div css={styles.drawerHeader}>
              <div css={[styles.logo, styles.drawerLogo]}>
                {logo_url ? (
                  <ExternalImage src={logo_url} alt="Custom Logo" />
                ) : (
                  <CoderIcon />
                )}
              </div>
            </div>
            <NavItems />
          </div>
        </Drawer>

        <NavLink css={styles.logo} to="/workspaces">
          {logo_url ? (
            <ExternalImage src={logo_url} alt="Custom Logo" />
          ) : (
            <CoderIcon fill="white" opacity={1} width={125} />
          )}
        </NavLink>

        <NavItems css={styles.desktopNavItems} />

        <div css={styles.navMenus}>
          {proxyContextValue && (
            <ProxyMenu proxyContextValue={proxyContextValue} />
          )}

          <DeploymentDropdown
            canViewAuditLog={canViewAuditLog}
            canViewOrganizations={canViewOrganizations}
            canViewDeployment={canViewDeployment}
            canViewAllUsers={canViewAllUsers}
            canViewHealth={canViewHealth}
          />

          {user && (
            <UserDropdown
              user={user}
              buildInfo={buildInfo}
              supportLinks={supportLinks}
              onSignOut={onSignOut}
            />
          )}
        </div>
      </div>
    </nav>
  );
};

const styles = {
  desktopNavItems: (theme) => css`
    display: none;

    ${theme.breakpoints.up("md")} {
      display: flex;
    }
  `,
  mobileMenuButton: (theme) => css`
    ${theme.breakpoints.up("md")} {
      display: none;
    }
  `,
  navMenus: (theme) => ({
    display: "flex",
    gap: 16,
    alignItems: "center",
    paddingRight: 16,

    [theme.breakpoints.up("md")]: {
      marginLeft: "auto",
    },
  }),
  wrapper: (theme) => css`
    position: relative;
    display: flex;
    justify-content: space-between;
    align-items: center;

    ${theme.breakpoints.up("md")} {
      justify-content: flex-start;
    }
  `,
  drawerHeader: {
    padding: 16,
    paddingTop: 32,
    paddingBottom: 32,
  },
  logo: (theme) => css`
    align-items: center;
    display: flex;
    height: ${navHeight}px;
    color: ${theme.palette.text.primary};
    padding: 16px;

    // svg is for the Coder logo, img is for custom images
    & svg,
    & img {
      height: 100%;
      object-fit: contain;
    }
  `,
  drawerLogo: {
    padding: 0,
    maxHeight: 40,
  },
  link: (theme) => css`
    align-items: center;
    color: ${theme.palette.text.secondary};
    display: flex;
    flex: 1;
    font-size: 16px;
    padding: 12px 16px;
    text-decoration: none;
    transition: background-color 0.15s ease-in-out;

    &.active {
      color: ${theme.palette.text.primary};
      font-weight: 500;
    }

    &:hover {
      background-color: ${theme.experimental.l2.hover.background};
    }

    ${theme.breakpoints.up("md")} {
      height: ${navHeight}px;
      padding: 0 24px;
    }
  `,
} satisfies Record<string, Interpolation<Theme>>;
