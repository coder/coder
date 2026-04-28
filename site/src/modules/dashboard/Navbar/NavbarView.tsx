import type { FC } from "react";
import { useQuery } from "react-query";
import { NavLink, useLocation } from "react-router";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { CoderIcon } from "#/components/Icons/CoderIcon";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import type { ProxyContextValue } from "#/contexts/ProxyContext";
import { useEmbeddedMetadata } from "#/hooks/useEmbeddedMetadata";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { NotificationsInbox } from "#/modules/notifications/NotificationsInbox/NotificationsInbox";
import { getPrereleaseFlag } from "#/utils/buildInfo";
import { cn } from "#/utils/cn";
import { DeploymentDropdown } from "./DeploymentDropdown";
import { MobileMenu } from "./MobileMenu";
import { ProxyMenu } from "./ProxyMenu";
import { SupportIcon } from "./SupportIcon";
import { UserDropdown } from "./UserDropdown/UserDropdown";

interface NavbarViewProps {
	logo_url?: string;
	user: TypesGen.User;
	buildInfo?: TypesGen.BuildInfoResponse;
	supportLinks: readonly TypesGen.LinkConfig[];
	onSignOut: () => void;
	canViewDeployment: boolean;
	canViewOrganizations: boolean;
	canViewAuditLog: boolean;
	canViewConnectionLog: boolean;
	canViewHealth: boolean;
	canViewAIBridge: boolean;
	canCreateChat: boolean;
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
	canViewConnectionLog,
	canViewAIBridge,
	canCreateChat,
	proxyContextValue,
}) => {
	const prerelease = getPrereleaseFlag(buildInfo);

	return (
		<div
			className={cn(
				"sticky top-0 bg-surface-primary z-40 border-0 border-b border-solid h-[72px] min-h-[72px] flex items-center leading-none px-6",
				prerelease &&
					cn(
						"[&:before]:content-[''] [&:before]:absolute [&:before]:left-0",
						"[&:before]:right-0 [&:before]:h-1 [&:before]:top-0",
						"[&:before]:bg-[repeating-linear-gradient(-45deg,_transparent,_transparent_4px,_hsl(var(--stripe-color)_/_0.5)_4px,_hsl(var(--stripe-color)_/_0.5)_8px)]",
					),
			)}
			style={{
				"--stripe-color":
					prerelease === "rc"
						? "var(--border-sky)"
						: prerelease === "devel"
							? "var(--content-warning)"
							: undefined,
			}}
		>
			<NavLink to="/workspaces">
				{logo_url ? (
					<ExternalImage className="h-7" src={logo_url} alt="Custom Logo" />
				) : (
					<CoderIcon className="h-7 w-7 fill-content-primary" />
				)}
			</NavLink>

			<NavItems className="ml-4" user={user} canCreateChat={canCreateChat} />

			{prerelease && buildInfo?.version && (
				<a
					href={buildInfo.external_url}
					target="_blank"
					rel="noreferrer"
					className="absolute top-0 left-1/2 -translate-x-1/2 no-underline z-10"
				>
					<Badge
						variant={prerelease === "rc" ? "info" : "warning"}
						size="sm"
						className="font-mono rounded-t-none border-t-0"
					>
						{buildInfo.version}
					</Badge>
				</a>
			)}

			<div className="flex items-center gap-3 ml-auto">
				{supportLinks.filter(isNavbarLink).map((link) => (
					<div key={link.name} className="hidden md:block">
						<SupportButton
							name={link.name}
							target={link.target}
							icon={link.icon}
						/>
					</div>
				))}

				{proxyContextValue && (
					<div className="hidden md:block">
						<ProxyMenu proxyContextValue={proxyContextValue} />
					</div>
				)}

				<div className="hidden md:block">
					<DeploymentDropdown
						canViewAuditLog={canViewAuditLog}
						canViewOrganizations={canViewOrganizations}
						canViewDeployment={canViewDeployment}
						canViewHealth={canViewHealth}
						canViewConnectionLog={canViewConnectionLog}
						canViewAIBridge={canViewAIBridge}
					/>
				</div>

				<NotificationsInbox
					fetchNotifications={API.getInboxNotifications}
					markAllAsRead={API.markAllInboxNotificationsAsRead}
					markNotificationAsRead={(notificationId) =>
						API.updateInboxNotificationReadStatus(notificationId, {
							is_read: true,
						})
					}
				/>

				<div className="hidden md:block">
					<UserDropdown
						user={user}
						buildInfo={buildInfo}
						supportLinks={supportLinks?.filter((link) => !isNavbarLink(link))}
						onSignOut={onSignOut}
					/>
				</div>

				<div className="md:hidden">
					<MobileMenu
						proxyContextValue={proxyContextValue}
						user={user}
						supportLinks={supportLinks}
						onSignOut={onSignOut}
						canViewAuditLog={canViewAuditLog}
						canViewConnectionLog={canViewConnectionLog}
						canViewOrganizations={canViewOrganizations}
						canViewDeployment={canViewDeployment}
						canViewHealth={canViewHealth}
					/>
				</div>
			</div>
		</div>
	);
};

interface NavItemsProps {
	className?: string;
	user: TypesGen.User;
	canCreateChat: boolean;
}

const NavItems: FC<NavItemsProps> = ({ className, user, canCreateChat }) => {
	const location = useLocation();

	return (
		<nav className={cn("flex items-center gap-4 h-full", className)}>
			<NavLink
				className={({ isActive }) => {
					if (location.pathname.startsWith("/@")) {
						isActive = true;
					}
					return cn(linkStyles.default, { [linkStyles.active]: isActive });
				}}
				to="/workspaces"
			>
				Workspaces
			</NavLink>
			<NavLink
				className={({ isActive }) => {
					return cn(linkStyles.default, { [linkStyles.active]: isActive });
				}}
				to="/templates"
			>
				Templates
			</NavLink>
			<TasksNavItem user={user} />
			<AgentsNavItem canCreateChat={canCreateChat} />
		</nav>
	);
};

type TasksNavItemProps = {
	user: TypesGen.User;
};

const TasksNavItem: FC<TasksNavItemProps> = ({ user }) => {
	const { metadata } = useEmbeddedMetadata();
	const canSeeTasks = Boolean(
		metadata["tasks-tab-visible"].value ||
			process.env.NODE_ENV === "development" ||
			process.env.STORYBOOK,
	);
	const filter: TypesGen.TasksFilter = {
		owner: user.username,
	};
	const { data: idleCount } = useQuery({
		queryKey: ["tasks", filter],
		queryFn: () => API.getTasks(filter),
		refetchInterval: 1_000 * 60,
		enabled: canSeeTasks,
		refetchOnWindowFocus: true,
		initialData: [],
		select: (data) =>
			data.filter((task) => task.current_state?.state === "idle").length,
	});

	if (!canSeeTasks) {
		return null;
	}

	return (
		<NavLink
			to="/tasks"
			className={({ isActive }) => {
				return cn(linkStyles.default, { [linkStyles.active]: isActive });
			}}
		>
			Tasks
			{idleCount > 0 && (
				<Tooltip>
					<TooltipTrigger asChild>
						<Badge
							variant="info"
							size="xs"
							className="ml-2"
							aria-label={idleTasksLabel(idleCount)}
						>
							{idleCount}
						</Badge>
					</TooltipTrigger>
					<TooltipContent>{idleTasksLabel(idleCount)}</TooltipContent>
				</Tooltip>
			)}
		</NavLink>
	);
};

function idleTasksLabel(count: number) {
	return `You have ${count} ${count === 1 ? "task" : "tasks"} waiting for input`;
}

const AgentsNavItem: FC<{ canCreateChat: boolean }> = ({ canCreateChat }) => {
	const { experiments, buildInfo } = useDashboard();
	const prerelease = getPrereleaseFlag(buildInfo);
	const experimentEnabled =
		experiments.includes("agents") || prerelease === "devel";

	if (!experimentEnabled || !canCreateChat) {
		return null;
	}

	return (
		<NavLink
			className={({ isActive }) => {
				return cn(linkStyles.default, { [linkStyles.active]: isActive });
			}}
			to="/agents"
		>
			Agents
		</NavLink>
	);
};

function isNavbarLink(link: TypesGen.LinkConfig): boolean {
	return link.location === "navbar";
}

interface SupportButtonProps {
	name: string;
	target: string;
	icon: string;
	location?: string;
}

const SupportButton: FC<SupportButtonProps> = ({ name, target, icon }) => {
	return (
		<Button asChild variant="outline">
			<a
				href={target}
				target="_blank"
				rel="noreferrer"
				className="inline-block"
			>
				{icon && <SupportIcon icon={icon} className="text-content-secondary" />}
				{name}
				<span className="sr-only"> (link opens in new tab)</span>
			</a>
		</Button>
	);
};
