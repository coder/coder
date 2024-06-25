import { cx } from "@emotion/css";
import type { FC, ReactNode } from "react";
import { Link, NavLink } from "react-router-dom";
import type { Organization } from "api/typesGenerated";
import { Sidebar as BaseSidebar } from "components/Sidebar/Sidebar";
import { Stack } from "components/Stack/Stack";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
import { type ClassName, useClassName } from "hooks/useClassName";
import { useOrganizationSettings } from "./OrganizationSettingsLayout";

export const Sidebar: FC = () => {
  const { currentOrganizationId, organizations } = useOrganizationSettings();

  // TODO: Do something nice to scroll to the active org.

  return (
    <BaseSidebar>
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

interface BloobProps {
  organization: Organization;
  active: boolean;
}

function urlForSubpage(organizationName: string, subpage: string = ""): string {
  return `/organizations/${organizationName}/${subpage}`;
}

export const OrganizationSettingsNavigation: FC<BloobProps> = ({
  organization,
  active,
}) => {
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
  active?: boolean;
  children?: ReactNode;
  icon: ReactNode;
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

  return (
    <Link to={href} className={cx([link, active && activeLink])}>
      <Stack alignItems="center" spacing={1.5} direction="row">
        {icon}
        {children}
      </Stack>
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
