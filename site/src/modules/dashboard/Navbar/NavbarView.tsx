import { API } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CoderIcon } from "components/Icons/CoderIcon";
import type { ProxyContextValue } from "contexts/ProxyContext";
import { NotificationsInbox } from "modules/notifications/NotificationsInbox/NotificationsInbox";
import type { FC } from "react";
import { NavLink, useLocation } from "react-router-dom";
import { cn } from "utils/cn";
import { DeploymentDropdown } from "./DeploymentDropdown";
import { MobileMenu } from "./MobileMenu";
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
	canViewAuditLog: boolean;
	canViewHealth: boolean;
	proxyContextValue?: ProxyContextValue;
}

const linkStyles = {
	default:
		"text-sm font-medium text-content-secondary no-underline block h-full px-2 flex items-center hover:text-content-primary transition-colors",
	active: "text-content-primary",
};

export const NavbarView: FC<NavbarViewProps> = ({
	user,
	logo_url,
	buildInfo,
	supportLinks,
	onSignOut,
	canViewDeployment,
	canViewOrganizations,
	canViewHealth,
	canViewAuditLog,
	proxyContextValue,
}) => {
	return (
		<div className="border-0 border-b border-solid h-[72px] flex items-center leading-none px-6">
			<NavLink to="/workspaces">
				{logo_url ? (
					<ExternalImage className="h-7" src={logo_url} alt="Custom Logo" />
				) : (
					<CoderIcon className="h-7 w-7 fill-content-primary" />
				)}
			</NavLink>

			<NavItems className="ml-4" />

			<div className="hidden md:flex items-center gap-3 ml-auto">
				{proxyContextValue && (
					<ProxyMenu proxyContextValue={proxyContextValue} />
				)}

				<DeploymentDropdown
					canViewAuditLog={canViewAuditLog}
					canViewOrganizations={canViewOrganizations}
					canViewDeployment={canViewDeployment}
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

			<div className="ml-auto flex items-center gap-3 md:hidden">
				<NotificationsInbox
					fetchNotifications={API.getInboxNotifications}
					markAllAsRead={() => {
						throw new Error("Function not implemented.");
					}}
					markNotificationAsRead={(notificationId) =>
						API.updateInboxNotificationReadStatus(notificationId, {
							is_read: true,
						})
					}
				/>

				<MobileMenu
					proxyContextValue={proxyContextValue}
					user={user}
					supportLinks={supportLinks}
					onSignOut={onSignOut}
					canViewAuditLog={canViewAuditLog}
					canViewOrganizations={canViewOrganizations}
					canViewDeployment={canViewDeployment}
					canViewHealth={canViewHealth}
				/>
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
					return cn(linkStyles.default, isActive ? linkStyles.active : "");
				}}
				to="/workspaces"
			>
				Workspaces
			</NavLink>
			<NavLink
				className={({ isActive }) => {
					return cn(linkStyles.default, isActive ? linkStyles.active : "");
				}}
				to="/templates"
			>
				Templates
			</NavLink>
		</nav>
	);
};
