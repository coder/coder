import Drawer from "@mui/material/Drawer";
import IconButton from "@mui/material/IconButton";
import Divider from "@mui/material/Divider";
import Skeleton from "@mui/material/Skeleton";
import Menu from "@mui/material/Menu";
import Button from "@mui/material/Button";
import MenuItem from "@mui/material/MenuItem";
import KeyboardArrowDownOutlined from "@mui/icons-material/KeyboardArrowDownOutlined";
import MenuIcon from "@mui/icons-material/Menu";
import { css, type Interpolation, type Theme, useTheme } from "@emotion/react";
import { type FC, type ReactNode, useRef, useState } from "react";
import { NavLink, useLocation, useNavigate } from "react-router-dom";
import { BUTTON_SM_HEIGHT, navHeight } from "theme/constants";
import type * as TypesGen from "api/typesGenerated";
import type { ProxyContextValue } from "contexts/ProxyContext";
import { displayError } from "components/GlobalSnackbar/utils";
import { ProxyStatusLatency } from "components/ProxyStatusLatency/ProxyStatusLatency";
import { CoderIcon } from "components/Icons/CoderIcon";
import { usePermissions } from "hooks/usePermissions";
import { UserDropdown } from "./UserDropdown/UserDropdown";
import { visuallyHidden } from "@mui/utils";
import { Abbr } from "components/Abbr/Abbr";

export const USERS_LINK = `/users?filter=${encodeURIComponent(
  "status:active",
)}`;

export interface NavbarViewProps {
  logo_url?: string;
  user?: TypesGen.User;
  buildInfo?: TypesGen.BuildInfoResponse;
  supportLinks?: TypesGen.LinkConfig[];
  onSignOut: () => void;
  canViewAuditLog: boolean;
  canViewDeployment: boolean;
  canViewAllUsers: boolean;
  canViewHealth: boolean;
  proxyContextValue?: ProxyContextValue;
}

export const Language = {
  workspaces: "Workspaces",
  templates: "Templates",
  users: "Users",
  audit: "Audit",
  deployment: "Deployment",
};

interface NavItemsProps {
  children?: ReactNode;
  className?: string;
  canViewAuditLog: boolean;
  canViewDeployment: boolean;
  canViewAllUsers: boolean;
  canViewHealth: boolean;
}

const NavItems: FC<NavItemsProps> = ({
  className,
  canViewAuditLog,
  canViewDeployment,
  canViewAllUsers,
  canViewHealth,
}) => {
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
      {canViewAllUsers && (
        <NavLink css={styles.link} to={USERS_LINK}>
          {Language.users}
        </NavLink>
      )}
      {canViewAuditLog && (
        <NavLink css={styles.link} to="/audit">
          {Language.audit}
        </NavLink>
      )}
      {canViewDeployment && (
        <NavLink css={styles.link} to="/deployment/general">
          {Language.deployment}
        </NavLink>
      )}
      {canViewHealth && (
        <NavLink css={styles.link} to="/health">
          Health
        </NavLink>
      )}
    </nav>
  );
};

export const NavbarView: FC<NavbarViewProps> = ({
  user,
  logo_url,
  buildInfo,
  supportLinks,
  onSignOut,
  canViewAuditLog,
  canViewDeployment,
  canViewAllUsers,
  canViewHealth,
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
                  <img src={logo_url} alt="Custom Logo" />
                ) : (
                  <CoderIcon />
                )}
              </div>
            </div>
            <NavItems
              canViewAuditLog={canViewAuditLog}
              canViewDeployment={canViewDeployment}
              canViewAllUsers={canViewAllUsers}
              canViewHealth={canViewHealth}
            />
          </div>
        </Drawer>

        <NavLink css={styles.logo} to="/workspaces">
          {logo_url ? (
            <img src={logo_url} alt="Custom Logo" />
          ) : (
            <CoderIcon fill="white" opacity={1} width={125} />
          )}
        </NavLink>

        <NavItems
          css={styles.desktopNavItems}
          canViewAuditLog={canViewAuditLog}
          canViewDeployment={canViewDeployment}
          canViewAllUsers={canViewAllUsers}
          canViewHealth={canViewHealth}
        />

        <div css={styles.navMenus}>
          {proxyContextValue && (
            <ProxyMenu proxyContextValue={proxyContextValue} />
          )}
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

interface ProxyMenuProps {
  proxyContextValue: ProxyContextValue;
}

const ProxyMenu: FC<ProxyMenuProps> = ({ proxyContextValue }) => {
  const theme = useTheme();
  const buttonRef = useRef<HTMLButtonElement>(null);
  const [isOpen, setIsOpen] = useState(false);
  const [refetchDate, setRefetchDate] = useState<Date>();
  const selectedProxy = proxyContextValue.proxy.proxy;
  const refreshLatencies = proxyContextValue.refetchProxyLatencies;
  const closeMenu = () => setIsOpen(false);
  const navigate = useNavigate();
  const latencies = proxyContextValue.proxyLatencies;
  const isLoadingLatencies = Object.keys(latencies).length === 0;
  const isLoading = proxyContextValue.isLoading || isLoadingLatencies;
  const permissions = usePermissions();

  const proxyLatencyLoading = (proxy: TypesGen.Region): boolean => {
    if (!refetchDate) {
      // Only show loading if the user manually requested a refetch
      return false;
    }

    // Only show a loading spinner if:
    //  - A latency exists. This means the latency was fetched at some point, so
    //    the loader *should* be resolved.
    //  - The proxy is healthy. If it is not, the loader might never resolve.
    //  - The latency reported is older than the refetch date. This means the
    //    latency is stale and we should show a loading spinner until the new
    //    latency is fetched.
    const latency = latencies[proxy.id];
    return proxy.healthy && latency !== undefined && latency.at < refetchDate;
  };

  if (isLoading) {
    return (
      <Skeleton
        width="110px"
        height={BUTTON_SM_HEIGHT}
        css={{ borderRadius: "9999px", transform: "none" }}
      />
    );
  }

  return (
    <>
      <Button
        ref={buttonRef}
        onClick={() => setIsOpen(true)}
        size="small"
        endIcon={<KeyboardArrowDownOutlined />}
        css={{
          borderRadius: "999px",
          "& .MuiSvgIcon-root": { fontSize: 14 },
        }}
      >
        <span css={{ ...visuallyHidden }}>
          Latency for {selectedProxy?.display_name ?? "your region"}
        </span>

        {selectedProxy ? (
          <div css={{ display: "flex", gap: 8, alignItems: "center" }}>
            <div css={{ width: 16, height: 16, lineHeight: 0 }}>
              <img
                // Empty alt text used because we don't want to double up on
                // screen reader announcements from visually-hidden span
                alt=""
                src={selectedProxy.icon_url}
                css={{
                  objectFit: "contain",
                  width: "100%",
                  height: "100%",
                }}
              />
            </div>

            <ProxyStatusLatency
              latency={latencies?.[selectedProxy.id]?.latencyMS}
              isLoading={proxyLatencyLoading(selectedProxy)}
            />
          </div>
        ) : (
          "Select Proxy"
        )}
      </Button>

      <Menu
        open={isOpen}
        anchorEl={buttonRef.current}
        onClick={closeMenu}
        onClose={closeMenu}
        css={{ "& .MuiMenu-paper": { paddingTop: 8, paddingBottom: 8 } }}
        // autoFocus here does not affect modal focus; it affects whether the
        // first item in the list will get auto-focus when the menu opens. Have
        // to turn this off because otherwise, screen readers will skip over all
        // the descriptive text and will only have access to the latency options
        autoFocus={false}
      >
        <div
          css={{
            width: "100%",
            maxWidth: "320px",
            fontSize: 14,
            padding: 16,
            lineHeight: "140%",
          }}
        >
          <h4
            autoFocus
            tabIndex={-1}
            css={{
              fontSize: "inherit",
              fontWeight: 600,
              lineHeight: "inherit",
              margin: 0,
              marginBottom: 4,
            }}
          >
            Select a region nearest to you
          </h4>

          <p
            css={{
              fontSize: 13,
              color: theme.palette.text.secondary,
              lineHeight: "inherit",
              marginTop: 0.5,
            }}
          >
            Workspace proxies improve terminal and web app connections to
            workspaces. This does not apply to{" "}
            <Abbr title="Command-Line Interface" pronunciation="initialism">
              CLI
            </Abbr>{" "}
            connections. A region must be manually selected, otherwise the
            default primary region will be used.
          </p>
        </div>

        <Divider css={{ borderColor: theme.palette.divider }} />

        {proxyContextValue.proxies
          ?.sort((a, b) => {
            const latencyA = latencies?.[a.id]?.latencyMS ?? Infinity;
            const latencyB = latencies?.[b.id]?.latencyMS ?? Infinity;
            return latencyA - latencyB;
          })
          .map((proxy) => (
            <MenuItem
              key={proxy.id}
              selected={proxy.id === selectedProxy?.id}
              css={{ fontSize: 14 }}
              onClick={() => {
                if (!proxy.healthy) {
                  displayError("Please select a healthy workspace proxy.");
                  closeMenu();
                  return;
                }

                proxyContextValue.setProxy(proxy);
                closeMenu();
              }}
            >
              <div
                css={{
                  display: "flex",
                  gap: 24,
                  alignItems: "center",
                  width: "100%",
                }}
              >
                <div css={{ width: 14, height: 14, lineHeight: 0 }}>
                  <img
                    src={proxy.icon_url}
                    alt=""
                    css={{
                      objectFit: "contain",
                      width: "100%",
                      height: "100%",
                    }}
                  />
                </div>

                {proxy.display_name}

                <ProxyStatusLatency
                  latency={latencies?.[proxy.id]?.latencyMS}
                  isLoading={proxyLatencyLoading(proxy)}
                />
              </div>
            </MenuItem>
          ))}

        <Divider css={{ borderColor: theme.palette.divider }} />

        {Boolean(permissions.editWorkspaceProxies) && (
          <MenuItem
            css={{ fontSize: 14 }}
            onClick={() => {
              navigate("deployment/workspace-proxies");
            }}
          >
            Proxy settings
          </MenuItem>
        )}

        <MenuItem
          css={{ fontSize: 14 }}
          onClick={(e) => {
            // Stop the menu from closing
            e.stopPropagation();
            // Refresh the latencies.
            const refetchDate = refreshLatencies();
            setRefetchDate(refetchDate);
          }}
        >
          Refresh Latencies
        </MenuItem>
      </Menu>
    </>
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
