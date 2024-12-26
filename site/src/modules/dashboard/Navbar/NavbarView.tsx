import type * as TypesGen from "api/typesGenerated";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CoderIcon } from "components/Icons/CoderIcon";
import type { ProxyContextValue } from "contexts/ProxyContext";
import type { FC } from "react";
import { NavLink, useLocation } from "react-router-dom";
import { DeploymentDropdown } from "./DeploymentDropdown";
import { ProxyMenu } from "./ProxyMenu";
import { UserDropdown } from "./UserDropdown/UserDropdown";
import { cn } from "utils/cn";
import { Button } from "components/Button/Button";
import { ChevronRightIcon, MenuIcon } from "lucide-react";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { Avatar } from "components/Avatar/Avatar";
import { Latency } from "components/Latency/Latency";

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

const mobileDropdownItemClassName =
	"px-9 h-[60px] border-0 border-b border-solid";

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

			<MobileMenu proxyContextValue={proxyContextValue} user={user} />
		</div>
	);
};

type MobileMenuProps = {
	proxyContextValue?: ProxyContextValue;
	user?: TypesGen.User;
};

const MobileMenu: FC<MobileMenuProps> = ({ proxyContextValue, user }) => {
	const selectedProxy = proxyContextValue?.proxy.proxy;
	const latency = selectedProxy
		? proxyContextValue?.proxyLatencies[selectedProxy?.id]
		: undefined;

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					aria-label="Open Menu"
					size="icon"
					variant="ghost"
					className="ml-auto md:hidden"
				>
					<MenuIcon />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent className="w-screen border-0 p-0" sideOffset={17}>
				{selectedProxy && (
					<DropdownMenuItem className={mobileDropdownItemClassName}>
						Workspace proxy settings:
						<span className="leading-[0px] flex items-center gap-1">
							<img
								className="w-4 h-4"
								src={selectedProxy.icon_url}
								alt={selectedProxy.name}
							/>
							{latency && <Latency latency={latency.latencyMS} />}
						</span>
						<ChevronRightIcon className="ml-auto" />
					</DropdownMenuItem>
				)}
				<DropdownMenuItem className={mobileDropdownItemClassName}>
					Admin settings
					<ChevronRightIcon className="ml-auto" />
				</DropdownMenuItem>
				<DropdownMenuItem className={mobileDropdownItemClassName}>
					Docs
				</DropdownMenuItem>
				<DropdownMenuItem className={mobileDropdownItemClassName}>
					<Avatar
						src={user?.avatar_url}
						fallback={user?.name || user?.username}
					/>
					User settings
					<ChevronRightIcon className="ml-auto" />
				</DropdownMenuItem>
			</DropdownMenuContent>
		</DropdownMenu>
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
				Workspaces
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
				Templates
			</NavLink>
		</nav>
	);
};
