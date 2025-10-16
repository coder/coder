import { API } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { CoderIcon } from "components/Icons/CoderIcon";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import type { ProxyContextValue } from "contexts/ProxyContext";
import { useWebpushNotifications } from "contexts/useWebpushNotifications";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { NotificationsInbox } from "modules/notifications/NotificationsInbox/NotificationsInbox";
import type { FC } from "react";
import { useQuery } from "react-query";
import { NavLink, useLocation } from "react-router";
import { cn } from "utils/cn";
import { DeploymentDropdown } from "./DeploymentDropdown";
import { MobileMenu } from "./MobileMenu";
import { ProxyMenu } from "./ProxyMenu";
import { SupportButtons } from "./SupportButtons";
import { UserDropdown } from "./UserDropdown/UserDropdown";

interface NavbarViewProps {
	logo_url?: string;
	user: TypesGen.User;
	buildInfo?: TypesGen.BuildInfoResponse;
	supportLinks?: readonly TypesGen.LinkConfig[];
	onSignOut: () => void;
	canViewDeployment: boolean;
	canViewOrganizations: boolean;
	canViewAuditLog: boolean;
	canViewConnectionLog: boolean;
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
	canViewConnectionLog,
	proxyContextValue,
}) => {
	const webPush = useWebpushNotifications();

	return (
		<div className="border-0 border-b border-solid h-[72px] min-h-[72px] flex items-center leading-none px-6">
			<NavLink to="/workspaces">
				{logo_url ? (
					<ExternalImage className="h-7" src={logo_url} alt="Custom Logo" />
				) : (
					<CoderIcon className="h-7 w-7 fill-content-primary" />
				)}
			</NavLink>

			<NavItems className="ml-4" user={user} />

			<div className="flex items-center gap-3 ml-auto">
				{proxyContextValue && (
					<div className="hidden md:block">
						<ProxyMenu proxyContextValue={proxyContextValue} />
					</div>
				)}

				{supportLinks && (
					<SupportButtons
						supportLinks={supportLinks.filter((l) => isNavbarLink(l))}
					/>
				)}

				<div className="hidden md:block">
					<DeploymentDropdown
						canViewAuditLog={canViewAuditLog}
						canViewOrganizations={canViewOrganizations}
						canViewDeployment={canViewDeployment}
						canViewHealth={canViewHealth}
						canViewConnectionLog={canViewConnectionLog}
					/>
				</div>

				{webPush.enabled ? (
					webPush.subscribed ? (
						<Button
							variant="outline"
							disabled={webPush.loading}
							onClick={webPush.unsubscribe}
						>
							Disable WebPush
						</Button>
					) : (
						<Button
							variant="outline"
							disabled={webPush.loading}
							onClick={webPush.subscribe}
						>
							Enable WebPush
						</Button>
					)
				) : null}

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
						supportLinks={supportLinks?.filter((link) => !isNavbarLink(link))}
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
}

const NavItems: FC<NavItemsProps> = ({ className, user }) => {
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
	const filter = {
		username: user.username,
	};
	const { data: idleCount } = useQuery({
		queryKey: ["tasks", filter],
		queryFn: () => API.experimental.getTasks(filter),
		refetchInterval: 1_000 * 60,
		enabled: canSeeTasks,
		refetchOnWindowFocus: true,
		initialData: [],
		select: (data) =>
			data.filter((task) => task.workspace.latest_app_status?.state === "idle")
				.length,
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
				<TooltipProvider>
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
				</TooltipProvider>
			)}
		</NavLink>
	);
};

function idleTasksLabel(count: number) {
	return `You have ${count} ${count === 1 ? "task" : "tasks"} waiting for input`;
}

function isNavbarLink(link: TypesGen.LinkConfig): boolean {
	return link.location === "navbar";
}
