import Drawer from "@mui/material/Drawer";
import IconButton from "@mui/material/IconButton";
import Divider from "@mui/material/Divider";
import Skeleton from "@mui/material/Skeleton";
import Box from "@mui/material/Box";
import Menu from "@mui/material/Menu";
import Button from "@mui/material/Button";
import MenuItem from "@mui/material/MenuItem";
import KeyboardArrowDownOutlined from "@mui/icons-material/KeyboardArrowDownOutlined";
import MenuIcon from "@mui/icons-material/Menu";
import { css, type Interpolation, type Theme, useTheme } from "@emotion/react";
import { type FC, type ReactNode, useRef, useState } from "react";
import { NavLink, useLocation, useNavigate } from "react-router-dom";
import { colors } from "theme/colors";
import type * as TypesGen from "api/typesGenerated";
import { BUTTON_SM_HEIGHT, navHeight } from "theme/constants";
import { ProxyContextValue } from "contexts/ProxyContext";
import { displayError } from "components/GlobalSnackbar/utils";
import { CoderIcon } from "components/Icons/CoderIcon";
import { ProxyStatusLatency } from "components/ProxyStatusLatency/ProxyStatusLatency";
import { usePermissions } from "hooks/usePermissions";
import { UserDropdown } from "./UserDropdown/UserDropdown";

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
  proxyContextValue?: ProxyContextValue;
}

export const Language = {
  workspaces: "Workspaces",
  templates: "Templates",
  users: "Users",
  audit: "Audit",
  deployment: "Deployment",
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
  item: {
    padding: 0,
  },
  link: (theme) => css`
    align-items: center;
    color: ${colors.gray[6]};
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
      background-color: ${theme.palette.action.hover};
    }

    ${theme.breakpoints.up("md")} {
      height: ${navHeight}px;
      padding: 0 24px;
    }
  `,
} satisfies Record<string, Interpolation<Theme>>;

interface NavItemsProps {
  children?: ReactNode;
  className?: string;
  canViewAuditLog: boolean;
  canViewDeployment: boolean;
  canViewAllUsers: boolean;
}

const NavItems: React.FC<NavItemsProps> = (props) => {
  const { className, canViewAuditLog, canViewDeployment, canViewAllUsers } =
    props;
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
  proxyContextValue,
}) => {
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);

  return (
    <nav
      css={(theme) => ({
        height: navHeight,
        background: theme.palette.background.paper,
        borderBottom: `1px solid ${theme.palette.divider}`,
      })}
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
        />

        <Box
          display="flex"
          marginLeft={{ md: "auto" }}
          gap={2}
          alignItems="center"
          paddingRight={2}
        >
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
        </Box>
      </div>
    </nav>
  );
};

const ProxyMenu: FC<{ proxyContextValue: ProxyContextValue }> = ({
  proxyContextValue,
}) => {
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

    const latency = latencies?.[proxy.id];
    // Only show a loading spinner if:
    //  - A latency exists. This means the latency was fetched at some point, so the
    //    loader *should* be resolved.
    //  - The proxy is healthy. If it is not, the loader might never resolve.
    //  - The latency reported is older than the refetch date. This means the latency
    //    is stale and we should show a loading spinner until the new latency is
    //    fetched.
    if (proxy.healthy && latency && latency.at < refetchDate) {
      return true;
    }

    return false;
  };

  if (isLoading) {
    return (
      <Skeleton
        width="110px"
        height={BUTTON_SM_HEIGHT}
        sx={{ borderRadius: "9999px", transform: "none" }}
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
        sx={{
          borderRadius: "999px",
          "& .MuiSvgIcon-root": { fontSize: 14 },
        }}
      >
        {selectedProxy ? (
          <Box display="flex" gap={1} alignItems="center">
            <Box width={16} height={16} lineHeight={0}>
              <Box
                component="img"
                src={selectedProxy.icon_url}
                alt=""
                sx={{ objectFit: "contain" }}
                width="100%"
                height="100%"
              />
            </Box>
            <ProxyStatusLatency
              latency={latencies?.[selectedProxy.id]?.latencyMS}
              isLoading={proxyLatencyLoading(selectedProxy)}
            />
          </Box>
        ) : (
          "Select Proxy"
        )}
      </Button>
      <Menu
        open={isOpen}
        anchorEl={buttonRef.current}
        onClick={closeMenu}
        onClose={closeMenu}
        sx={{ "& .MuiMenu-paper": { py: 1 } }}
      >
        <Box
          sx={{
            w: "100%",
            fontSize: 14,
            padding: 2,
            maxWidth: "320px",
            lineHeight: "140%",
          }}
        >
          <h4
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
            css={(theme) => ({
              fontSize: 13,
              color: theme.palette.text.secondary,
              lineHeight: "inherit",
              marginTop: 0.5,
            })}
          >
            Workspace proxies improve terminal and web app connections to
            workspaces. This does not apply to CLI connections. A region must be
            manually selected, otherwise the default primary region will be
            used.
          </p>
        </Box>
        <Divider sx={{ borderColor: (theme) => theme.palette.divider }} />
        {proxyContextValue.proxies
          ?.sort((a, b) => {
            const latencyA = latencies?.[a.id]?.latencyMS ?? Infinity;
            const latencyB = latencies?.[b.id]?.latencyMS ?? Infinity;
            return latencyA - latencyB;
          })
          .map((proxy) => (
            <MenuItem
              onClick={() => {
                if (!proxy.healthy) {
                  displayError("Please select a healthy workspace proxy.");
                  closeMenu();
                  return;
                }

                proxyContextValue.setProxy(proxy);
                closeMenu();
              }}
              key={proxy.id}
              selected={proxy.id === selectedProxy?.id}
              sx={{
                fontSize: 14,
              }}
            >
              <Box display="flex" gap={3} alignItems="center" width="100%">
                <Box width={14} height={14} lineHeight={0}>
                  <Box
                    component="img"
                    src={proxy.icon_url}
                    alt=""
                    sx={{ objectFit: "contain" }}
                    width="100%"
                    height="100%"
                  />
                </Box>
                {proxy.display_name}
                <ProxyStatusLatency
                  latency={latencies?.[proxy.id]?.latencyMS}
                  isLoading={proxyLatencyLoading(proxy)}
                />
              </Box>
            </MenuItem>
          ))}
        <Divider sx={{ borderColor: (theme) => theme.palette.divider }} />
        {Boolean(permissions.editWorkspaceProxies) && (
          <MenuItem
            sx={{ fontSize: 14 }}
            onClick={() => {
              navigate("deployment/workspace-proxies");
            }}
          >
            Proxy settings
          </MenuItem>
        )}
        <MenuItem
          sx={{ fontSize: 14 }}
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
