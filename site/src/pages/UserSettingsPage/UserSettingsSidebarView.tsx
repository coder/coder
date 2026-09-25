import type { FC } from "react";
import { Link, useLocation } from "react-router";
import type { User } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { useSidebarContext } from "#/components/Sidebar/SidebarContext";
import { SidebarGroup } from "#/components/Sidebar/SidebarGroup";
import {
	SidebarHeader,
	SidebarHeaderTitle,
} from "#/components/Sidebar/SidebarHeader";
import { SidebarNavLink } from "#/components/Sidebar/SidebarNavLink";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type UserSettingsLink = {
	label: string;
	href: string;
	visible: boolean;
};

type UserSettingsGroup = {
	label: string;
	items: UserSettingsLink[];
};

const isRouteActive = (pathname: string, href: string) =>
	pathname === href || pathname.startsWith(`${href}/`);

/** Header for the user settings sidebar. */
export const UserSettingsSidebarHeader: FC = () => (
	<SidebarHeader>
		<SidebarHeaderTitle>Your account</SidebarHeaderTitle>
	</SidebarHeader>
);

type UserSettingsSidebarViewProps = {
	user: User;
	showSchedulePage: boolean;
	showOAuth2Page: boolean;
};

/** User settings navigation. Collapsed, it shows only the user's avatar. */
export const UserSettingsSidebarView: FC<UserSettingsSidebarViewProps> = ({
	user,
	showSchedulePage,
	showOAuth2Page,
}) => {
	const { pathname } = useLocation();
	const { collapsed, expand } = useSidebarContext();
	const displayName = user.name || user.username;

	const groups: UserSettingsGroup[] = [
		{
			label: "General",
			items: [
				{
					label: "Account information",
					href: "/settings/account",
					visible: true,
				},
				{ label: "Appearance", href: "/settings/appearance", visible: true },
				{
					label: "Notifications",
					href: "/settings/notifications",
					visible: true,
				},
				{
					label: "Schedule",
					href: "/settings/schedule",
					visible: showSchedulePage,
				},
			],
		},
		{
			label: "Connected accounts",
			items: [
				{
					label: "External authentication",
					href: "/settings/external-auth",
					visible: true,
				},
				{
					label: "OAuth2 applications",
					href: "/settings/oauth2-provider",
					visible: showOAuth2Page,
				},
			],
		},
		{
			label: "Credentials",
			items: [
				{ label: "SSH keys", href: "/settings/ssh-keys", visible: true },
				{ label: "Secrets", href: "/settings/secrets", visible: true },
				{ label: "Tokens", href: "/settings/tokens", visible: true },
				{ label: "Security", href: "/settings/security", visible: true },
			],
		},
	];

	if (collapsed) {
		return (
			<TooltipProvider>
				<Tooltip delayDuration={0}>
					<TooltipTrigger asChild>
						<Link
							to="/settings/account"
							onClick={expand}
							aria-label={displayName}
							className="flex items-center justify-center w-10 h-10 rounded-md no-underline hover:bg-surface-secondary"
						>
							<Avatar
								size="lg"
								fallback={user.username}
								src={user.avatar_url}
							/>
						</Link>
					</TooltipTrigger>
					<TooltipContent side="right">{displayName}</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		);
	}

	return (
		<div className="flex flex-col gap-4">
			<div className="flex items-center gap-2 px-1 py-1">
				<Avatar size="lg" fallback={user.username} src={user.avatar_url} />
				<div className="flex min-w-0 flex-1 flex-col">
					<span className="truncate text-sm text-content-primary">
						{displayName}
					</span>
					<span className="truncate text-xs text-content-secondary">
						{user.email}
					</span>
				</div>
			</div>
			{groups.map((group) => (
				<SidebarGroup
					key={group.label}
					label={group.label}
					active={group.items.some(
						(item) => item.visible && isRouteActive(pathname, item.href),
					)}
				>
					{group.items
						.filter((item) => item.visible)
						.map((item) => (
							<SidebarNavLink key={item.href} href={item.href}>
								{item.label}
							</SidebarNavLink>
						))}
				</SidebarGroup>
			))}
		</div>
	);
};
