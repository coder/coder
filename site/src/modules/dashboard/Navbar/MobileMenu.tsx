import type * as TypesGen from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "components/Collapsible/Collapsible";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { FeatureStageBadge } from "components/FeatureStageBadge/FeatureStageBadge";
import { displayError } from "components/GlobalSnackbar/utils";
import { Latency } from "components/Latency/Latency";
import type { ProxyContextValue } from "contexts/ProxyContext";
import {
	ChevronRightIcon,
	CircleHelpIcon,
	MenuIcon,
	XIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import { Link } from "react-router-dom";
import { cn } from "utils/cn";
import { sortProxiesByLatency } from "./proxyUtils";

const itemStyles = {
	default: "px-9 h-10 no-underline",
	sub: "pl-12",
	open: "text-content-primary",
};

type MobileMenuPermissions = {
	canViewDeployment: boolean;
	canViewOrganizations: boolean;
	canViewAuditLog: boolean;
	canViewHealth: boolean;
};

type MobileMenuProps = MobileMenuPermissions & {
	proxyContextValue?: ProxyContextValue;
	user?: TypesGen.User;
	supportLinks?: readonly TypesGen.LinkConfig[];
	onSignOut: () => void;
	isDefaultOpen?: boolean; // Useful for storybook
};

export const MobileMenu: FC<MobileMenuProps> = ({
	isDefaultOpen,
	proxyContextValue,
	user,
	supportLinks,
	onSignOut,
	...permissions
}) => {
	const [open, setOpen] = useState(isDefaultOpen);
	const hasSomePermission = Object.values(permissions).some((p) => p);

	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
			{open && (
				<div className="fixed inset-0 top-[72px] backdrop-blur-sm z-10 bg-surface-primary/50" />
			)}
			<DropdownMenuTrigger asChild>
				<Button
					aria-label={open ? "Close menu" : "Open menu"}
					size="lg"
					variant="subtle"
					className="ml-auto md:hidden"
				>
					{open ? <XIcon /> : <MenuIcon />}
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				className="w-screen border-0 border-b border-solid p-0 py-2"
				sideOffset={17}
			>
				<ProxySettingsSub proxyContextValue={proxyContextValue} />

				{hasSomePermission && (
					<>
						<DropdownMenuSeparator />
						<AdminSettingsSub {...permissions} />
					</>
				)}
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
					className={cn(itemStyles.default, open ? itemStyles.open : "")}
					onClick={(e) => {
						e.preventDefault();
						setOpen((prev) => !prev);
					}}
				>
					Workspace proxy settings:
					<span className="leading-none flex items-center gap-1">
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
								className={cn(itemStyles.default, itemStyles.sub)}
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
					className={cn(itemStyles.default, itemStyles.sub)}
				>
					<Link to="/deployment/workspace-proxies">Proxy settings</Link>
				</DropdownMenuItem>
				<DropdownMenuItem
					className={cn(itemStyles.default, itemStyles.sub)}
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

const AdminSettingsSub: FC<MobileMenuPermissions> = ({
	canViewDeployment,
	canViewOrganizations,
	canViewAuditLog,
	canViewHealth,
}) => {
	const [open, setOpen] = useState(false);

	return (
		<Collapsible open={open} onOpenChange={setOpen}>
			<CollapsibleTrigger asChild>
				<DropdownMenuItem
					className={cn(itemStyles.default, open ? itemStyles.open : "")}
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
				{canViewDeployment && (
					<DropdownMenuItem
						asChild
						className={cn(itemStyles.default, itemStyles.sub)}
					>
						<Link to="/deployment/general">Deployment</Link>
					</DropdownMenuItem>
				)}
				{canViewOrganizations && (
					<DropdownMenuItem
						asChild
						className={cn(itemStyles.default, itemStyles.sub)}
					>
						<Link to="/organizations">Organizations</Link>
					</DropdownMenuItem>
				)}
				{canViewAuditLog && (
					<DropdownMenuItem
						asChild
						className={cn(itemStyles.default, itemStyles.sub)}
					>
						<Link to="/audit">Audit logs</Link>
					</DropdownMenuItem>
				)}
				{canViewHealth && (
					<DropdownMenuItem
						asChild
						className={cn(itemStyles.default, itemStyles.sub)}
					>
						<Link to="/health">Healthcheck</Link>
					</DropdownMenuItem>
				)}
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
					className={cn(itemStyles.default, open ? itemStyles.open : "")}
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
					className={cn(itemStyles.default, itemStyles.sub)}
				>
					<Link to="/settings/account">Account</Link>
				</DropdownMenuItem>
				<DropdownMenuItem
					className={cn(itemStyles.default, itemStyles.sub)}
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
								className={cn(itemStyles.default, itemStyles.sub)}
							>
								<a
									href={includeOrigin(l.target)}
									target="_blank"
									rel="noreferrer"
								>
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

const includeOrigin = (target: string): string => {
	if (target.startsWith("/")) {
		const baseUrl = window.location.origin;
		return `${baseUrl}${target}`;
	}
	return target;
};
