import Drawer from "@mui/material/Drawer";
import IconButton from "@mui/material/IconButton";
import List from "@mui/material/List";
import ListItem from "@mui/material/ListItem";
import { makeStyles } from "@mui/styles";
import MenuIcon from "@mui/icons-material/Menu";
import { CoderIcon } from "components/Icons/CoderIcon";
import { FC, useRef, useState } from "react";
import { NavLink, useLocation, useNavigate } from "react-router-dom";
import { colors } from "theme/colors";
import * as TypesGen from "api/typesGenerated";
import { navHeight } from "theme/constants";
import { combineClasses } from "utils/combineClasses";
import { UserDropdown } from "./UserDropdown/UserDropdown";
import Box from "@mui/material/Box";
import Menu from "@mui/material/Menu";
import Button from "@mui/material/Button";
import MenuItem from "@mui/material/MenuItem";
import KeyboardArrowDownOutlined from "@mui/icons-material/KeyboardArrowDownOutlined";
import { ProxyContextValue } from "contexts/ProxyContext";
import { displayError } from "components/GlobalSnackbar/utils";
import Divider from "@mui/material/Divider";
import Skeleton from "@mui/material/Skeleton";
import { BUTTON_SM_HEIGHT } from "theme/theme";
import { ProxyStatusLatency } from "components/ProxyStatusLatency/ProxyStatusLatency";
import { usePermissions } from "hooks/usePermissions";
import Typography from "@mui/material/Typography";

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

const NavItems: React.FC<
  React.PropsWithChildren<{
    className?: string;
    canViewAuditLog: boolean;
    canViewDeployment: boolean;
    canViewAllUsers: boolean;
  }>
> = ({ className, canViewAuditLog, canViewDeployment, canViewAllUsers }) => {
  const styles = useStyles();
  const location = useLocation();

  return (
    <List className={combineClasses([styles.navItems, className])}>
      <ListItem button className={styles.item}>
        <NavLink
          className={combineClasses([
            styles.link,
            location.pathname.startsWith("/@") && "active",
          ])}
          to="/workspaces"
        >
          {Language.workspaces}
        </NavLink>
      </ListItem>
      <ListItem button className={styles.item}>
        <NavLink className={styles.link} to="/templates">
          {Language.templates}
        </NavLink>
      </ListItem>
      {canViewAllUsers && (
        <ListItem button className={styles.item}>
          <NavLink className={styles.link} to={USERS_LINK}>
            {Language.users}
          </NavLink>
        </ListItem>
      )}
      {canViewAuditLog && (
        <ListItem button className={styles.item}>
          <NavLink className={styles.link} to="/audit">
            {Language.audit}
          </NavLink>
        </ListItem>
      )}
      {canViewDeployment && (
        <ListItem button className={styles.item}>
          <NavLink className={styles.link} to="/deployment/general">
            {Language.deployment}
          </NavLink>
        </ListItem>
      )}
    </List>
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
  const styles = useStyles();
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);

  return (
    <nav className={styles.root}>
      <div className={styles.wrapper}>
        <IconButton
          aria-label="Open menu"
          className={styles.mobileMenuButton}
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
          <div className={styles.drawer}>
            <div className={styles.drawerHeader}>
              <div className={combineClasses([styles.logo, styles.drawerLogo])}>
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

        <NavLink className={styles.logo} to="/workspaces">
          {logo_url ? (
            <img src={logo_url} alt="Custom Logo" />
          ) : (
            <CoderIcon fill="white" opacity={1} width={125} />
          )}
        </NavLink>

        <NavItems
          className={styles.desktopNavItems}
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
        width="160px"
        height={BUTTON_SM_HEIGHT}
        sx={{ borderRadius: "4px", transform: "none" }}
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
          borderRadius: "4px",
          "& .MuiSvgIcon-root": { fontSize: 14 },
        }}
      >
        {selectedProxy ? (
          <Box display="flex" gap={2} alignItems="center">
            <Box width={14} height={14} lineHeight={0}>
              <Box
                component="img"
                src={selectedProxy.icon_url}
                alt=""
                sx={{ objectFit: "contain" }}
                width="100%"
                height="100%"
              />
            </Box>
            {selectedProxy.display_name}
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
          <Typography
            component="h4"
            sx={{
              fontSize: "inherit",
              fontWeight: 600,
              lineHeight: "inherit",
              margin: 0,
            }}
          >
            Select a region nearest to you
          </Typography>
          <Typography
            component="p"
            sx={{
              fontSize: 13,
              color: (theme) => theme.palette.text.secondary,
              lineHeight: "inherit",
              marginTop: 0.5,
            }}
          >
            Workspace proxies improve terminal and web app connections to
            workspaces. This does not apply to CLI connections. A region must be
            manually selected, otherwise the default primary region will be
            used.
          </Typography>
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

const useStyles = makeStyles((theme) => ({
  displayInitial: {
    display: "initial",
  },
  root: {
    height: navHeight,
    background: theme.palette.background.paper,
    borderBottom: `1px solid ${theme.palette.divider}`,
  },
  wrapper: {
    position: "relative",
    display: "flex",
    justifyContent: "space-between",
    alignItems: "center",
    [theme.breakpoints.up("md")]: {
      justifyContent: "flex-start",
    },
  },
  drawer: {
    width: 250,
  },
  drawerHeader: {
    padding: theme.spacing(2),
    paddingTop: theme.spacing(4),
    paddingBottom: theme.spacing(4),
  },
  navItems: {
    padding: 0,
  },
  desktopNavItems: {
    display: "none",
    [theme.breakpoints.up("md")]: {
      display: "flex",
    },
  },
  mobileMenuButton: {
    [theme.breakpoints.up("md")]: {
      display: "none",
    },
  },
  logo: {
    alignItems: "center",
    display: "flex",
    height: navHeight,
    color: theme.palette.text.primary,
    padding: theme.spacing(2),
    // svg is for the Coder logo, img is for custom images
    "& svg, & img": {
      height: "100%",
      objectFit: "contain",
    },
  },
  drawerLogo: {
    padding: 0,
    maxHeight: theme.spacing(5),
  },
  title: {
    flex: 1,
    textAlign: "center",
  },
  item: {
    padding: 0,
  },
  link: {
    alignItems: "center",
    color: colors.gray[6],
    display: "flex",
    flex: 1,
    fontSize: 16,
    padding: `${theme.spacing(1.5)} ${theme.spacing(2)}`,
    textDecoration: "none",
    transition: "background-color 0.15s ease-in-out",

    "&:hover": {
      backgroundColor: theme.palette.action.hover,
    },

    // NavLink adds this class when the current route matches.
    "&.active": {
      color: theme.palette.text.primary,
      fontWeight: 500,
    },

    [theme.breakpoints.up("md")]: {
      height: navHeight,
      padding: `0 ${theme.spacing(3)}`,
    },
  },
}));
