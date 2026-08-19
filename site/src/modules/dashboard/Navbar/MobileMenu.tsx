import {
	ChevronRightIcon,
	CircleHelpIcon,
	MenuIcon,
	RadioIcon,
	XIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import type * as TypesGen from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { Latency } from "#/components/Latency/Latency";
import type { ProxyContextValue } from "#/contexts/ProxyContext";
import { cn } from "#/utils/cn";
import { getLatencyColor } from "#/utils/latency";
import {
	AdminSettingsItems,
	type AdminSettingsPermissions,
	canViewAdminSettings,
} from "./AdminSettings";
import { sortProxiesByLatency } from "./proxyUtils";
import { restrictedNavMessages } from "./RestrictedNavItem";

const itemStyles = {
	default: "px-9 h-10 no-underline",
	sub: "pl-12",
	open: "text-content-primary",
};

type MobileMenuProps = {
	proxyContextValue?: ProxyContextValue;
	adminPermissions: AdminSettingsPermissions;
	canViewWorkspaces: boolean;
	canViewTemplates: boolean;
	user?: TypesGen.User;
	supportLinks?: readonly TypesGen.LinkConfig[];
	onSignOut: () => void;
	isDefaultOpen?: boolean; // Useful for storybook
};

export const MobileMenu: FC<MobileMenuProps> = ({
	adminPermissions,
	proxyContextValue,
	canViewWorkspaces,
	canViewTemplates,
	user,
	supportLinks,
	onSignOut,
	isDefaultOpen,
}) => {
	const [open, setOpen] = useState(isDefaultOpen);
	const navItems = [
		{
			label: "Workspaces",
			to: "/workspaces",
			enabled: canViewWorkspaces,
			message: restrictedNavMessages.workspaces,
		},
		{
			label: "Templates",
			to: "/templates",
			enabled: canViewTemplates,
			message: restrictedNavMessages.templates,
		},
	];

	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
			{open && (
				<div className="fixed inset-0 top-[72px] z-10 bg-surface-primary" />
			)}
			<DropdownMenuTrigger asChild>
				<Button
					aria-label={open ? "Close menu" : "Open menu"}
					size="icon-lg"
					variant="subtle"
				>
					{open ? <XIcon /> : <MenuIcon />}
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				className="w-screen border-0 border-b border-solid p-0 py-2"
				sideOffset={17}
			>
				{navItems.map(({ label, to, enabled, message }) =>
					enabled ? (
						<DropdownMenuItem
							key={label}
							asChild
							className={itemStyles.default}
						>
							<Link to={to}>{label}</Link>
						</DropdownMenuItem>
					) : (
						<RestrictedMenuItem key={label} label={label} message={message} />
					),
				)}
				<DropdownMenuItem asChild className={itemStyles.default}>
					<Link to="/agents">Agents</Link>
				</DropdownMenuItem>
				{canViewWorkspaces && (
					<>
						<DropdownMenuSeparator />
						<ProxySettingsSub proxyContextValue={proxyContextValue} />
					</>
				)}

				{canViewAdminSettings(adminPermissions) && (
					<>
						<DropdownMenuSeparator />
						<AdminSettingsSub permissions={adminPermissions} />
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

type RestrictedMenuItemProps = {
	label: string;
	message: string;
};

const RestrictedMenuItem: FC<RestrictedMenuItemProps> = ({
	label,
	message,
}) => {
	const [open, setOpen] = useState(false);

	return (
		<Collapsible open={open} onOpenChange={setOpen}>
			<CollapsibleTrigger asChild>
				<DropdownMenuItem
					aria-label={`${label} (unavailable)`}
					className={cn(itemStyles.default, open && itemStyles.open)}
					onSelect={(event) => event.preventDefault()}
					onClick={(event) => {
						event.preventDefault();
						setOpen((prev) => !prev);
					}}
				>
					<span className="opacity-50">{label}</span>
					<CircleHelpIcon className="ml-auto opacity-50" />
				</DropdownMenuItem>
			</CollapsibleTrigger>
			<CollapsibleContent className="px-9 pb-2 text-xs text-content-secondary">
				{message}
			</CollapsibleContent>
		</Collapsible>
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
					className={cn(itemStyles.default, open && itemStyles.open)}
					onClick={(e) => {
						e.preventDefault();
						setOpen((prev) => !prev);
					}}
				>
					Workspace proxy settings:
					<span className="leading-none flex items-center gap-1">
						<span className="sr-only">
							Latency for {selectedProxy.display_name || selectedProxy.name}
						</span>
						<RadioIcon
							aria-hidden="true"
							className={cn("size-4", getLatencyColor(latency?.latencyMS))}
						/>
						<Latency
							className={
								latency?.latencyMS ? "text-content-primary" : undefined
							}
							latency={latency?.latencyMS}
						/>
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
										toast.error("Failed to select proxy.", {
											description: "Please select a healthy workspace proxy.",
										});
										return;
									}

									proxyContextValue.setProxy(p);
									setOpen(false);
								}}
							>
								<ExternalImage
									className="size-4"
									src={p.icon_url}
									alt={p.name}
								/>
								{p.display_name || p.name}
								{latency ? (
									<Latency className="ml-auto" latency={latency.latencyMS} />
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
					onClick={(event) => {
						event.stopPropagation();
						proxyContextValue.refetchProxyLatencies();
					}}
				>
					Refresh latencies
				</DropdownMenuItem>
			</CollapsibleContent>
		</Collapsible>
	);
};

type AdminSettingsSubProps = {
	permissions: AdminSettingsPermissions;
};

const AdminSettingsSub: FC<AdminSettingsSubProps> = ({ permissions }) => {
	const [open, setOpen] = useState(false);

	return (
		<Collapsible open={open} onOpenChange={setOpen}>
			<CollapsibleTrigger asChild>
				<DropdownMenuItem
					className={cn(itemStyles.default, open && itemStyles.open)}
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
				<AdminSettingsItems
					itemClassName={cn(itemStyles.default, itemStyles.sub)}
					permissions={permissions}
				/>
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
					className={cn(itemStyles.default, open && itemStyles.open)}
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

export const includeOrigin = (target: string): string => {
	if (target.startsWith("/")) {
		const baseUrl = location.origin;
		return `${baseUrl}${target}`;
	}
	return target;
};
