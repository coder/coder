import { cx } from "@emotion/css";
import type { Interpolation, Theme } from "@emotion/react";
import AddIcon from "@mui/icons-material/Add";
import SettingsIcon from "@mui/icons-material/Settings";
import type { FC, ReactNode } from "react";
import { Link, NavLink } from "react-router-dom";
import type { AuthorizationResponse, Organization } from "api/typesGenerated";
import { Loader } from "components/Loader/Loader";
import { Sidebar as BaseSidebar } from "components/Sidebar/Sidebar";
import { Stack } from "components/Stack/Stack";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { type ClassName, useClassName } from "hooks/useClassName";
import { linkToAuditing, linkToUsers, withFilter } from "modules/navigation";

interface SidebarProps {
  /**
   * The active org if an org is being viewed.  If there is no active
   * organization, assume one of the deployment settings pages are being viewed.
   */
  activeOrganization: Organization | undefined;
  /**
   * The permissions for the active org or undefined if still fetching (or if
   * there is no active org).
   */
  activeOrgPermissions: AuthorizationResponse | undefined;
  /** The list of organizations or undefined if still fetching. */
  organizations: Organization[] | undefined;
  /** Site-wide permissions. */
  permissions: AuthorizationResponse;
}

/**
 * A combined deployment settings and organization menu.
 */
export const SidebarView: FC<SidebarProps> = (props) => {
  // TODO: Do something nice to scroll to the active org.
  return (
    <BaseSidebar>
      <header css={styles.sidebarHeader}>Deployment</header>
      <DeploymentSettingsNavigation
        active={!props.activeOrganization}
        permissions={props.permissions}
      />
      {props.organizations ? (
        <>
          <header css={styles.sidebarHeader}>Organizations</header>
          {props.permissions.createOrganization && (
            <SidebarNavItem
              active="auto"
              href="/organizations/new"
              icon={<AddIcon />}
            >
              New organization
            </SidebarNavItem>
          )}
          {props.organizations.map((org) => {
            const orgActive =
              Boolean(props.activeOrganization) &&
              org.name === props.activeOrganization?.name;
            return (
              <OrganizationSettingsNavigation
                key={org.id}
                organization={org}
                permissions={orgActive ? props.activeOrgPermissions : undefined}
                active={orgActive}
              />
            );
          })}
        </>
      ) : (
        <Loader />
      )}
    </BaseSidebar>
  );
};

interface DeploymentSettingsNavigationProps {
  /** Whether a deployment setting page is being viewed. */
  active: boolean;
  /** Site-wide permissions. */
  permissions: AuthorizationResponse;
}

/**
 * Displays navigation for deployment settings.  If active, highlight the main
 * menu heading.
 *
 * Menu items are shown based on the permissions.  If organizations can be
 * viewed, groups are skipped since they will show under each org instead.
 */
const DeploymentSettingsNavigation: FC<DeploymentSettingsNavigationProps> = (
  props,
) => {
  return (
    <div css={{ paddingBottom: 12 }}>
      <SidebarNavItem
        active={props.active}
        href={
          props.permissions.viewDeploymentValues
            ? "/deployment/general"
            : "/deployment/workspace-proxies"
        }
        // 24px matches the width of the organization icons, and the component
        // is smart enough to keep the icon itself square. It looks too big if
        // it's 24x24.
        icon={<SettingsIcon css={{ width: 24, height: 20 }} />}
      >
        Deployment
      </SidebarNavItem>
      {props.active && (
        <Stack spacing={0.5} css={{ marginBottom: 8, marginTop: 8 }}>
          {props.permissions.viewDeploymentValues && (
            <SidebarNavSubItem href="general">General</SidebarNavSubItem>
          )}
          {props.permissions.viewAllLicenses && (
            <SidebarNavSubItem href="licenses">Licenses</SidebarNavSubItem>
          )}
          {props.permissions.editDeploymentValues && (
            <SidebarNavSubItem href="appearance">Appearance</SidebarNavSubItem>
          )}
          {props.permissions.viewDeploymentValues && (
            <SidebarNavSubItem href="userauth">
              User Authentication
            </SidebarNavSubItem>
          )}
          {props.permissions.viewDeploymentValues && (
            <SidebarNavSubItem href="external-auth">
              External Authentication
            </SidebarNavSubItem>
          )}
          {/* Not exposing this yet since token exchange is not finished yet.
          <SidebarNavSubItem href="oauth2-provider/ap>
            OAuth2 Applications
          </SidebarNavSubItem>*/}
          {props.permissions.viewDeploymentValues && (
            <SidebarNavSubItem href="network">Network</SidebarNavSubItem>
          )}
          {/* All users can view workspace regions.  */}
          <SidebarNavSubItem href="workspace-proxies">
            Workspace Proxies
          </SidebarNavSubItem>
          {props.permissions.viewDeploymentValues && (
            <SidebarNavSubItem href="security">Security</SidebarNavSubItem>
          )}
          {props.permissions.viewDeploymentValues && (
            <SidebarNavSubItem href="observability">
              Observability
            </SidebarNavSubItem>
          )}
          {props.permissions.viewAllUsers && (
            <SidebarNavSubItem href={linkToUsers.slice(1)}>
              Users
            </SidebarNavSubItem>
          )}
          {props.permissions.viewAnyAuditLog && (
            <SidebarNavSubItem href={linkToAuditing.slice(1)}>
              Auditing
            </SidebarNavSubItem>
          )}
        </Stack>
      )}
    </div>
  );
};

function urlForSubpage(organizationName: string, subpage: string = ""): string {
  return `/organizations/${organizationName}/${subpage}`;
}

interface OrganizationSettingsNavigationProps {
  active: boolean;
  organization: Organization;
  permissions: AuthorizationResponse | undefined;
}

/**
 * Displays navigation for an organization.
 *
 * If inactive, no sub-menu items will be shown, just the organization name.
 *
 * If active, it will show a loader until the permissions are defined, then the
 * sub-menu items are shown as appropriate.
 */
const OrganizationSettingsNavigation: FC<
  OrganizationSettingsNavigationProps
> = (props) => {
  return (
    <>
      <SidebarNavItem
        active={props.active}
        href={urlForSubpage(props.organization.name)}
        icon={
          <UserAvatar
            key={props.organization.id}
            size="sm"
            username={props.organization.display_name}
            avatarURL={props.organization.icon}
          />
        }
      >
        {props.organization.display_name}
      </SidebarNavItem>
      {props.active && !props.permissions && <Loader />}
      {props.active && props.permissions && (
        <Stack spacing={0.5} css={{ marginBottom: 8, marginTop: 8 }}>
          {props.permissions.editOrganization && (
            <SidebarNavSubItem
              end
              href={urlForSubpage(props.organization.name)}
            >
              Organization settings
            </SidebarNavSubItem>
          )}
          {props.permissions.viewMembers && (
            <SidebarNavSubItem
              href={urlForSubpage(props.organization.name, "members")}
            >
              Members
            </SidebarNavSubItem>
          )}
          {props.permissions.viewGroups && (
            <SidebarNavSubItem
              href={urlForSubpage(props.organization.name, "groups")}
            >
              Groups
            </SidebarNavSubItem>
          )}
          {/* For now redirect to the site-wide audit page with the organization
              pre-filled into the filter.  Based on user feedback we might want
              to serve a copy of the audit page or even delete this link. */}
          {props.permissions.auditOrganization && (
            <SidebarNavSubItem
              href={`/deployment${withFilter(
                linkToAuditing,
                `organization:${props.organization.name}`,
              )}`}
            >
              Auditing
            </SidebarNavSubItem>
          )}
        </Stack>
      )}
    </>
  );
};

interface SidebarNavItemProps {
  active?: boolean | "auto";
  children?: ReactNode;
  icon?: ReactNode;
  href: string;
}

const SidebarNavItem: FC<SidebarNavItemProps> = ({
  active,
  children,
  href,
  icon,
}) => {
  const link = useClassName(classNames.link, []);
  const activeLink = useClassName(classNames.activeLink, []);

  const content = (
    <Stack alignItems="center" spacing={1.5} direction="row">
      {icon}
      {children}
    </Stack>
  );

  if (active === "auto") {
    return (
      <NavLink
        to={href}
        className={({ isActive }) => cx([link, isActive && activeLink])}
      >
        {content}
      </NavLink>
    );
  }

  return (
    <Link to={href} className={cx([link, active && activeLink])}>
      {content}
    </Link>
  );
};

interface SidebarNavSubItemProps {
  children?: ReactNode;
  href: string;
  end?: boolean;
}

const SidebarNavSubItem: FC<SidebarNavSubItemProps> = ({
  children,
  href,
  end,
}) => {
  const link = useClassName(classNames.subLink, []);
  const activeLink = useClassName(classNames.activeSubLink, []);

  return (
    <NavLink
      end={end}
      to={href}
      className={({ isActive }) => cx([link, isActive && activeLink])}
    >
      {children}
    </NavLink>
  );
};

const styles = {
  sidebarHeader: {
    textTransform: "uppercase",
    letterSpacing: "0.15em",
    fontSize: 11,
    fontWeight: 500,
    paddingBottom: 4,
  },
} satisfies Record<string, Interpolation<Theme>>;

const classNames = {
  link: (css, theme) => css`
    color: inherit;
    display: block;
    font-size: 14px;
    text-decoration: none;
    padding: 10px 12px 10px 16px;
    border-radius: 4px;
    transition: background-color 0.15s ease-in-out;
    position: relative;

    &:hover {
      background-color: ${theme.palette.action.hover};
    }

    border-left: 3px solid transparent;
  `,

  activeLink: (css, theme) => css`
    border-left-color: ${theme.palette.primary.main};
    border-top-left-radius: 0;
    border-bottom-left-radius: 0;
  `,

  subLink: (css, theme) => css`
    color: inherit;
    text-decoration: none;

    display: block;
    font-size: 13px;
    margin-left: 44px;
    padding: 4px 12px;
    border-radius: 4px;
    transition: background-color 0.15s ease-in-out;
    margin-bottom: 1px;
    position: relative;

    &:hover {
      background-color: ${theme.palette.action.hover};
    }
  `,

  activeSubLink: (css) => css`
    font-weight: 600;
  `,
} satisfies Record<string, ClassName>;
