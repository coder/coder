import { cn } from "cn";
import {
	ChevronRightIcon,
	CircleHelpIcon,
	MenuIcon,
	RadioIcon,
	XIcon,
} from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
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
import { toast } from "#/components/Toaster/toast";
import type { ProxyContextValue } from "#/contexts/ProxyContext";
import { getLatencyColor } from "#/utils/latency";
import {
	AdminSettingsItems,
	type AdminSettingsPermissions,
	canViewAdminSettings,
} from "./AdminSettings";
import { sortProxiesByLatency } from "./proxyUtils";

const itemStyles = {
	default: "px-9 h-10 no-underline",
	sub: "pl-12",
	open: "text-content-primary",
};

type MobileMenuProps = {
	proxyContextValue?: ProxyContextValue;
	adminPermissions: AdminSettingsPermissions;
	user?: TypesGen.User;
	supportLinks?: readonly TypesGen.LinkConfig[];
	onSignOut: () => void;
	isDefaultOpen?: boolean; // Useful for storybook
};

export const MobileMenu: React.FC<MobileMenuProps> = ({
	adminPermissions,
	proxyContextValue,
	user,
	supportLinks,
	onSignOut,
	isDefaultOpen,
}) => {
	const [open, setOpen] = useState(isDefaultOpen);

	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
			{open && (
				<div className="fixed inset-0 top-[72px] z-10 bg-surface-primary" />
			)}
			<DropdownMenuTrigger
				render={
					<Button
						aria-label={open ? "Close menu" : "Open menu"}
						size="icon-lg"
						variant="subtle"
					/>
				}
			>
				{open ? <XIcon /> : <MenuIcon />}
			</DropdownMenuTrigger>
			<DropdownMenuContent
				className="w-screen border-0 border-b border-solid p-0 py-2"
				sideOffset={17}
			>
				<DropdownMenuItem
					render={<Link to="/workspaces" />}
					className={itemStyles.default}
				>
					Workspaces
				</DropdownMenuItem>
				<DropdownMenuItem
					render={<Link to="/templates" />}
					className={itemStyles.default}
				>
					Templates
				</DropdownMenuItem>
				<DropdownMenuItem
					render={<Link to="/agents" />}
					className={itemStyles.default}
				>
					Agents
				</DropdownMenuItem>
				<DropdownMenuSeparator />
				<ProxySettingsSub proxyContextValue={proxyContextValue} />

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

type ProxySettingsSubProps = {
	proxyContextValue?: ProxyContextValue;
};

const ProxySettingsSub: React.FC<ProxySettingsSubProps> = ({
	proxyContextValue,
}) => {
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
					closeOnClick={false}
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
								closeOnClick={false}
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
					render={<Link to="/deployment/workspace-proxies" />}
					className={cn(itemStyles.default, itemStyles.sub)}
				>
					Proxy settings
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

const AdminSettingsSub: React.FC<AdminSettingsSubProps> = ({ permissions }) => {
	const [open, setOpen] = useState(false);

	return (
		<Collapsible open={open} onOpenChange={setOpen}>
			<CollapsibleTrigger asChild>
				<DropdownMenuItem
					className={cn(itemStyles.default, open && itemStyles.open)}
					closeOnClick={false}
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

const UserSettingsSub: React.FC<UserSettingsSubProps> = ({
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
					closeOnClick={false}
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
					render={<Link to="/settings/account" />}
					className={cn(itemStyles.default, itemStyles.sub)}
				>
					Account
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
								render={
									<a
										href={includeOrigin(l.target)}
										target="_blank"
										rel="noreferrer"
									>
										{l.name}
									</a>
								}
								className={cn(itemStyles.default, itemStyles.sub)}
							></DropdownMenuItem>
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
