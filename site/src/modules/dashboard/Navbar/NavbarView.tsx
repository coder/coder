import type * as TypesGen from "api/typesGenerated";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CoderIcon } from "components/Icons/CoderIcon";
import type { ProxyContextValue } from "contexts/ProxyContext";
import type { FC } from "react";
import { NavLink, useLocation } from "react-router-dom";
import { cn } from "utils/cn";
import { DeploymentDropdown } from "./DeploymentDropdown";
import { MobileMenu } from "./MobileMenu";
import { ProxyMenu } from "./ProxyMenu";
import { UserDropdown } from "./UserDropdown/UserDropdown";
import { API } from "api/api";

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

			<div className=" hidden md:flex items-center gap-3 ml-auto">
				{proxyContextValue && (
					<ProxyMenu proxyContextValue={proxyContextValue} />
				)}

				<button onClick={() => {
					Notification.requestPermission().then(async (permission) => {
						if (permission === "granted") {
							const registration = await navigator.serviceWorker.ready;
							registration.pushManager.subscribe({
								userVisibleOnly: true,
								applicationServerKey: buildInfo?.notifications_vapid_public_key,
							}).then((subscription) => {
								const json = subscription.toJSON()
								API.updateUserBrowserNotificationSubscription(user?.id ?? "me", {
									subscription: {
										endpoint: json.endpoint!,
									keys: json.keys! as any,
									},
								}).then(() => {
									console.log("Subscribed to browser notifications");
								})
							});
						}
					});
				}}>
					Subscribe
				</button>

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
