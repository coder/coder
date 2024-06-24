import { cx } from "@emotion/css";
import AddIcon from "@mui/icons-material/Add";
import SettingsIcon from "@mui/icons-material/Settings";
import type { FC, ReactNode } from "react";
import { Link, NavLink, useLocation } from "react-router-dom";
import type { Organization } from "api/typesGenerated";
import { Sidebar as BaseSidebar } from "components/Sidebar/Sidebar";
import { Stack } from "components/Stack/Stack";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { type ClassName, useClassName } from "hooks/useClassName";
import { useOrganizationSettings } from "./ManagementSettingsLayout";

export const Sidebar: FC = () => {
  const { currentOrganizationId, organizations } = useOrganizationSettings();

  // TODO: Do something nice to scroll to the active org.

  return (
    <BaseSidebar>
      <DeploymentSettingsNavigation />
      <header
        css={{
          textTransform: "uppercase",
          letterSpacing: "0.15em",
          fontSize: 11,
          fontWeight: 500,
          paddingBottom: 4,
        }}
      >
        Organizations
      </header>
      <SidebarNavItem
        active="auto"
        href="/organizations/new"
        icon={<AddIcon />}
      >
        New organization
      </SidebarNavItem>
      {organizations.map((organization) => (
        <OrganizationSettingsNavigation
          key={organization.id}
          organization={organization}
          active={organization.id === currentOrganizationId}
        />
      ))}
    </BaseSidebar>
  );
};

const DeploymentSettingsNavigation: FC = () => {
  const location = useLocation();
  const active = location.pathname.startsWith("/deployment");

  return (
    <div css={{ paddingBottom: 12 }}>
      <SidebarNavItem
        active={active}
        href="/deployment/general"
        // 24px matches the width of the organization icons, and the component is smart enough
        // to keep the icon itself square. It looks too big if it's 24x24.
        icon={<SettingsIcon css={{ width: 24, height: 20 }} />}
      >
        Deployment
      </SidebarNavItem>
      {active && (
        <Stack spacing={0.5} css={{ marginBottom: 8, marginTop: 8 }}>
          <SidebarNavSubItem href="general">General</SidebarNavSubItem>
          <SidebarNavSubItem href="licenses">Licenses</SidebarNavSubItem>
          <SidebarNavSubItem href="appearance">Appearance</SidebarNavSubItem>
          <SidebarNavSubItem href="userauth">
            User Authentication
          </SidebarNavSubItem>
          <SidebarNavSubItem href="external-auth">
            External Authentication
          </SidebarNavSubItem>
          {/* Not exposing this yet since token exchange is not finished yet.
          <SidebarNavSubItem href="oauth2-provider/ap>
            OAuth2 Applications
          </SidebarNavSubItem>*/}
          <SidebarNavSubItem href="network">Network</SidebarNavSubItem>
          <SidebarNavSubItem href="workspace-proxies">
            Workspace Proxies
          </SidebarNavSubItem>
          <SidebarNavSubItem href="security">Security</SidebarNavSubItem>
          <SidebarNavSubItem href="observability">
            Observability
          </SidebarNavSubItem>
          <SidebarNavSubItem href="/users">Users</SidebarNavSubItem>
          <SidebarNavSubItem href="/groups">Groups</SidebarNavSubItem>
        </Stack>
      )}
    </div>
  );
};

function urlForSubpage(organizationName: string, subpage: string = ""): string {
  return `/organizations/${organizationName}/${subpage}`;
}

interface OrganizationSettingsNavigationProps {
  organization: Organization;
  active: boolean;
}

export const OrganizationSettingsNavigation: FC<
  OrganizationSettingsNavigationProps
> = ({ organization, active }) => {
  return (
    <>
      <SidebarNavItem
        active={active}
        href={urlForSubpage(organization.name)}
        icon={
          <UserAvatar
            key={organization.id}
            size="sm"
            username={organization.display_name}
            avatarURL={organization.icon}
          />
        }
      >
        {organization.display_name}
      </SidebarNavItem>
      {active && (
        <Stack spacing={0.5} css={{ marginBottom: 8, marginTop: 8 }}>
          <SidebarNavSubItem href={urlForSubpage(organization.name)}>
            Organization settings
          </SidebarNavSubItem>
          <SidebarNavSubItem
            href={urlForSubpage(organization.name, "external-auth")}
          >
            External authentication
          </SidebarNavSubItem>
          <SidebarNavSubItem href={urlForSubpage(organization.name, "members")}>
            Members
          </SidebarNavSubItem>
          <SidebarNavSubItem href={urlForSubpage(organization.name, "groups")}>
            Groups
          </SidebarNavSubItem>
          <SidebarNavSubItem href={urlForSubpage(organization.name, "metrics")}>
            Metrics
          </SidebarNavSubItem>
          <SidebarNavSubItem
            href={urlForSubpage(organization.name, "auditing")}
          >
            Auditing
          </SidebarNavSubItem>
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

export const SidebarNavItem: FC<SidebarNavItemProps> = ({
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
}

export const SidebarNavSubItem: FC<SidebarNavSubItemProps> = ({
  children,
  href,
}) => {
  const link = useClassName(classNames.subLink, []);
  const activeLink = useClassName(classNames.activeSubLink, []);

  return (
    <NavLink
      end
      to={href}
      className={({ isActive }) => cx([link, isActive && activeLink])}
    >
      {children}
    </NavLink>
  );
};

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
    margin-left: 42px;
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
