import { type Interpolation, type Theme, css, useTheme } from "@emotion/react";
import MenuIcon from "@mui/icons-material/Menu";
import Drawer from "@mui/material/Drawer";
import IconButton from "@mui/material/IconButton";
import type * as TypesGen from "api/typesGenerated";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CoderIcon } from "components/Icons/CoderIcon";
import type { ProxyContextValue } from "contexts/ProxyContext";
import { type FC, useState } from "react";
import { Link, NavLink, useLocation } from "react-router-dom";
import { DeploymentDropdown } from "./DeploymentDropdown";
import { ProxyMenu } from "./ProxyMenu";
import { UserDropdown } from "./UserDropdown/UserDropdown";
import { cn } from "utils/cn";

export const Language = {
	workspaces: "Workspaces",
	templates: "Templates",
	users: "Users",
	audit: "Audit Logs",
	deployment: "Deployment",
};
export interface NavbarViewProps {
	logo_url?: string;
	user?: TypesGen.User;
	docsHref: string;
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

const linkClassNames = {
	default:
		"text-sm font-medium text-content-secondary no-underline block h-full px-2 flex items-center hover:text-content-primary transition-colors",
	active: "text-content-primary",
};

export const NavbarView: FC<NavbarViewProps> = ({
	user,
	logo_url,
	docsHref,
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
	const [isDrawerOpen, setIsDrawerOpen] = useState(false);

	return (
		<div className="border-0 border-b border-solid h-[72px] flex items-center leading-none px-6">
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
						<div css={["h-7", styles.drawerLogo]}>
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

			<NavLink to="/workspaces">
				{logo_url ? (
					<ExternalImage className="h-7" src={logo_url} alt="Custom Logo" />
				) : (
					<CoderIcon className="h-7 w-7 fill-content-primary" />
				)}
			</NavLink>

			<NavItems className="ml-4" />

			<div className="flex items-center gap-3 ml-auto">
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

				<a
					className={linkClassNames.default}
					href={docsHref}
					target="_blank"
					rel="noreferrer"
				>
					Docs
				</a>

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
	);
};

interface NavItemsProps {
	className?: string;
}

const NavItems: FC<NavItemsProps> = ({ className }) => {
	const location = useLocation();

	return (
		<nav className={cn("flex items-center gap-4 h-full", className)}>
			<NavLink
				className={({ isActive }) => {
					if (location.pathname.startsWith("/@")) {
						isActive = true;
					}
					return cn(
						linkClassNames.default,
						isActive ? linkClassNames.active : "",
					);
				}}
				to="/workspaces"
			>
				{Language.workspaces}
			</NavLink>
			<NavLink
				className={({ isActive }) => {
					return cn(
						linkClassNames.default,
						isActive ? linkClassNames.active : "",
					);
				}}
				to="/templates"
			>
				{Language.templates}
			</NavLink>
		</nav>
	);
};

const styles = {
	mobileMenuButton: (theme) => css`
    ${theme.breakpoints.up("md")} {
      display: none;
    }
  `,

	drawerHeader: {
		padding: 16,
		paddingTop: 32,
		paddingBottom: 32,
	},
	drawerLogo: {
		padding: 0,
		maxHeight: 40,
	},
} satisfies Record<string, Interpolation<Theme>>;
