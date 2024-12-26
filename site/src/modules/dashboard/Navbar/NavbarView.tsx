import type * as TypesGen from "api/typesGenerated";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CoderIcon } from "components/Icons/CoderIcon";
import type { ProxyContextValue } from "contexts/ProxyContext";
import { useState, type FC } from "react";
import { Link, NavLink, useLocation } from "react-router-dom";
import { DeploymentDropdown } from "./DeploymentDropdown";
import { ProxyMenu } from "./ProxyMenu";
import { UserDropdown } from "./UserDropdown/UserDropdown";
import { cn } from "utils/cn";
import { Button } from "components/Button/Button";
import {
	ChevronRightIcon,
	CircleHelpIcon,
	MenuIcon,
	XIcon,
} from "lucide-react";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { Avatar } from "components/Avatar/Avatar";
import { Latency } from "components/Latency/Latency";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "components/Collapsible/Collapsible";
import { sortProxiesByLatency } from "./proxyUtils";
import { displayError } from "components/GlobalSnackbar/utils";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";

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

const mobileDropdownItemClassNames = {
	default: "px-9 h-10 no-underline",
	sub: "pl-12",
	open: "text-content-primary",
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

			<MobileMenu
				proxyContextValue={proxyContextValue}
				user={user}
				supportLinks={supportLinks}
				docsHref={docsHref}
				onSignOut={onSignOut}
			/>
		</div>
	);
};

type MobileMenuProps = {
	proxyContextValue?: ProxyContextValue;
	user?: TypesGen.User;
	supportLinks?: readonly TypesGen.LinkConfig[];
	docsHref: string;
	onSignOut: () => void;
};

const MobileMenu: FC<MobileMenuProps> = ({
	proxyContextValue,
	user,
	supportLinks,
	docsHref,
	onSignOut,
}) => {
	const [open, setOpen] = useState(false);

	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
			{open && (
				<div className="fixed inset-0 top-[72px] backdrop-blur-sm z-10 bg-content-primary/50" />
			)}
			<DropdownMenuTrigger asChild>
				<Button
					aria-label={open ? "Close menu" : "Open menu"}
					size="icon"
					variant="ghost"
					className="ml-auto md:hidden [&_svg]:size-6"
				>
					{open ? <XIcon /> : <MenuIcon />}
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				className="w-screen border-0 border-b border-solid p-0 py-2"
				sideOffset={17}
			>
				<ProxySettingsSub proxyContextValue={proxyContextValue} />
				<DropdownMenuSeparator />
				<AdminSettingsSub />
				<DropdownMenuSeparator />
				<DropdownMenuItem
					asChild
					className={mobileDropdownItemClassNames.default}
				>
					<a href={docsHref} target="_blank" rel="noreferrer norefereer">
						Docs
					</a>
				</DropdownMenuItem>
				<DropdownMenuSeparator />
				<UserSettingsSub
					user={user}
					supportLinks={supportLinks}
					onSignOut={onSignOut}
				/>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

type ProxySettingsSubProps = {
	proxyContextValue?: ProxyContextValue;
};

const ProxySettingsSub: FC<ProxySettingsSubProps> = ({ proxyContextValue }) => {
	const selectedProxy = proxyContextValue?.proxy.proxy;
	const latency = selectedProxy
		? proxyContextValue?.proxyLatencies[selectedProxy?.id]
		: undefined;
	const [open, setOpen] = useState(false);

	if (!selectedProxy) {
		return null;
	}

	return (
		<Collapsible open={open} onOpenChange={setOpen}>
			<CollapsibleTrigger asChild>
				<DropdownMenuItem
					className={cn(
						mobileDropdownItemClassNames.default,
						open ? mobileDropdownItemClassNames.open : "",
					)}
					onClick={(e) => {
						e.preventDefault();
						setOpen((prev) => !prev);
					}}
				>
					Workspace proxy settings:
					<span className="leading-[0px] flex items-center gap-1">
						<img
							className="w-4 h-4"
							src={selectedProxy.icon_url}
							alt={selectedProxy.name}
						/>
						{latency && <Latency latency={latency.latencyMS} />}
					</span>
					<ChevronRightIcon
						className={cn("ml-auto", open ? "rotate-90" : "")}
					/>
				</DropdownMenuItem>
			</CollapsibleTrigger>
			<CollapsibleContent>
				{proxyContextValue.proxies &&
					sortProxiesByLatency(
						proxyContextValue.proxies,
						proxyContextValue.proxyLatencies,
					).map((p) => {
						const latency = proxyContextValue.proxyLatencies[p.id];
						return (
							<DropdownMenuItem
								className={cn(
									mobileDropdownItemClassNames.default,
									mobileDropdownItemClassNames.sub,
								)}
								key={p.id}
								onClick={(e) => {
									e.preventDefault();

									if (!p.healthy) {
										displayError("Please select a healthy workspace proxy.");
										return;
									}

									proxyContextValue.setProxy(p);
									setOpen(false);
								}}
							>
								<img className="w-4 h-4" src={p.icon_url} alt={p.name} />
								{p.display_name || p.name}
								{latency ? (
									<Latency latency={latency.latencyMS} />
								) : (
									<CircleHelpIcon className="ml-auto" />
								)}
							</DropdownMenuItem>
						);
					})}
				<DropdownMenuSeparator />
				<DropdownMenuItem
					asChild
					className={cn(
						mobileDropdownItemClassNames.default,
						mobileDropdownItemClassNames.sub,
					)}
				>
					<Link to="/deployment/workspace-proxies">Proxy settings</Link>
				</DropdownMenuItem>
				<DropdownMenuItem
					className={cn(
						mobileDropdownItemClassNames.default,
						mobileDropdownItemClassNames.sub,
					)}
					onClick={() => {
						proxyContextValue.refetchProxyLatencies();
					}}
				>
					Refresh latencies
				</DropdownMenuItem>
			</CollapsibleContent>
		</Collapsible>
	);
};

const AdminSettingsSub: FC = () => {
	const [open, setOpen] = useState(false);

	return (
		<Collapsible open={open} onOpenChange={setOpen}>
			<CollapsibleTrigger asChild>
				<DropdownMenuItem
					className={cn(
						mobileDropdownItemClassNames.default,
						open ? mobileDropdownItemClassNames.open : "",
					)}
					onClick={(e) => {
						e.preventDefault();
						setOpen((prev) => !prev);
					}}
				>
					Admin settings
					<ChevronRightIcon
						className={cn("ml-auto", open ? "rotate-90" : "")}
					/>
				</DropdownMenuItem>
			</CollapsibleTrigger>
			<CollapsibleContent>
				<DropdownMenuItem
					asChild
					className={cn(
						mobileDropdownItemClassNames.default,
						mobileDropdownItemClassNames.sub,
					)}
				>
					<Link to="/deployment/general">Deployment</Link>
				</DropdownMenuItem>
				<DropdownMenuItem
					asChild
					className={cn(
						mobileDropdownItemClassNames.default,
						mobileDropdownItemClassNames.sub,
					)}
				>
					<Link to="/organizations">
						Organizations
						<FeatureStageBadge
							contentType="beta"
							size="sm"
							showTooltip={false}
						/>
					</Link>
				</DropdownMenuItem>
				<DropdownMenuItem
					asChild
					className={cn(
						mobileDropdownItemClassNames.default,
						mobileDropdownItemClassNames.sub,
					)}
				>
					<Link to="/audit">Audit logs</Link>
				</DropdownMenuItem>
				<DropdownMenuItem
					asChild
					className={cn(
						mobileDropdownItemClassNames.default,
						mobileDropdownItemClassNames.sub,
					)}
				>
					<Link to="/health">Healthcheck</Link>
				</DropdownMenuItem>
			</CollapsibleContent>
		</Collapsible>
	);
};

type UserSettingsSubProps = {
	user?: TypesGen.User;
	supportLinks?: readonly TypesGen.LinkConfig[];
	onSignOut: () => void;
};

const UserSettingsSub: FC<UserSettingsSubProps> = ({
	user,
	supportLinks,
	onSignOut,
}) => {
	const [open, setOpen] = useState(false);

	return (
		<Collapsible open={open} onOpenChange={setOpen}>
			<CollapsibleTrigger asChild>
				<DropdownMenuItem
					className={cn(
						mobileDropdownItemClassNames.default,
						open ? mobileDropdownItemClassNames.open : "",
					)}
					onClick={(e) => {
						e.preventDefault();
						setOpen((prev) => !prev);
					}}
				>
					<Avatar
						src={user?.avatar_url}
						fallback={user?.name || user?.username}
					/>
					User settings
					<ChevronRightIcon
						className={cn("ml-auto", open ? "rotate-90" : "")}
					/>
				</DropdownMenuItem>
			</CollapsibleTrigger>
			<CollapsibleContent>
				<DropdownMenuItem
					asChild
					className={cn(
						mobileDropdownItemClassNames.default,
						mobileDropdownItemClassNames.sub,
					)}
				>
					<Link to="/settings/account">Account</Link>
				</DropdownMenuItem>
				<DropdownMenuItem
					asChild
					className={cn(
						mobileDropdownItemClassNames.default,
						mobileDropdownItemClassNames.sub,
					)}
					onClick={onSignOut}
				>
					Sign out
				</DropdownMenuItem>
				{supportLinks && (
					<>
						<DropdownMenuSeparator />
						{supportLinks?.map((l) => (
							<DropdownMenuItem
								key={l.name}
								asChild
								className={cn(
									mobileDropdownItemClassNames.default,
									mobileDropdownItemClassNames.sub,
								)}
							>
								<a href={l.target} target="_blank" rel="noreferrer">
									{l.name}
								</a>
							</DropdownMenuItem>
						))}
					</>
				)}
			</CollapsibleContent>
		</Collapsible>
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
