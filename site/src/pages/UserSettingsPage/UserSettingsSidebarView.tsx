import {
	BlocksIcon,
	FingerprintIcon,
	KeyRoundIcon,
	PanelLeftIcon,
	ShieldCheckIcon,
	UserIcon,
	UserLockIcon,
	VenetianMaskIcon,
} from "lucide-react";
import type { FC } from "react";
import { useLocation } from "react-router";
import type { User } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { SidebarAccordion } from "#/components/Sidebar/SidebarAccordion";
import { useSidebarContext } from "#/components/Sidebar/SidebarContext";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { SidebarNavLink } from "#/modules/management/SidebarNavLink";
import { SidebarTopLevelNavItem } from "#/modules/management/SidebarTopLevelNavItem";
import { useOpenSections } from "#/modules/management/useOpenSections";

const DEFAULT_OPEN_SECTIONS_STORAGE_KEY = "user-settings-sidebar-open-sections";
const GENERAL_SECTION = "general";
const GENERAL_ROUTES = [
	"/settings/account",
	"/settings/appearance",
	"/settings/notifications",
	"/settings/schedule",
];

interface UserSettingsSidebarHeaderProps {
	user: User;
}

/**
 * Pinned header for the user settings sidebar: the user's avatar, name,
 * and email beside the collapse toggle, over a full-bleed divider. The
 * collapsed rail shows only the avatar (which re-expands the sidebar)
 * and the toggle.
 */
export const UserSettingsSidebarHeader: FC<UserSettingsSidebarHeaderProps> = ({
	user,
}) => {
	const { collapsed, expand, toggle } = useSidebarContext();
	const displayName = user.name || user.username;

	if (collapsed) {
		return (
			<>
				<div className="flex flex-col gap-1 px-3 py-3">
					<TooltipProvider>
						<Tooltip delayDuration={0}>
							<TooltipTrigger asChild>
								<button
									type="button"
									onClick={expand}
									className="flex items-center justify-center w-10 h-10 rounded-md cursor-pointer bg-transparent border-none p-0 hover:bg-surface-secondary"
								>
									<Avatar
										size="lg"
										fallback={user.username}
										src={user.avatar_url}
									/>
								</button>
							</TooltipTrigger>
							<TooltipContent side="right">{displayName}</TooltipContent>
						</Tooltip>
					</TooltipProvider>
					<button
						type="button"
						onClick={toggle}
						className="group flex items-center justify-center w-10 h-10 rounded-md cursor-pointer bg-transparent border-none p-0"
					>
						<PanelLeftIcon className="size-4 text-content-secondary group-hover:text-content-primary transition-colors" />
					</button>
				</div>
				<div className="h-px shrink-0 bg-border" />
			</>
		);
	}

	return (
		<>
			<div className="flex items-center gap-2 px-3 py-3 pl-4">
				<Avatar size="lg" fallback={user.username} src={user.avatar_url} />
				<div className="flex min-w-0 flex-1 flex-col">
					<span className="truncate text-sm text-content-primary">
						{displayName}
					</span>
					<span className="truncate text-xs text-content-secondary">
						{user.email}
					</span>
				</div>
				<button
					type="button"
					onClick={toggle}
					className="group flex h-10 shrink-0 items-center bg-transparent border-none cursor-pointer p-0 pr-1"
				>
					<PanelLeftIcon className="size-4 text-content-secondary group-hover:text-content-primary transition-colors" />
				</button>
			</div>
			<div className="h-px shrink-0 bg-border" />
		</>
	);
};

interface UserSettingsSidebarViewProps {
	/** Schedule page is entitlement gated. */
	showSchedulePage: boolean;
	/** OAuth2 applications page is behind an experiment or dev builds. */
	showOAuth2Page: boolean;
	/** Overridable so stories do not share persisted accordion state. */
	openSectionsStorageKey?: string;
}

/**
 * Navigation for the user settings area: a General accordion with the
 * account-level pages, followed by flat icon links.
 */
export const UserSettingsSidebarView: FC<UserSettingsSidebarViewProps> = ({
	showSchedulePage,
	showOAuth2Page,
	openSectionsStorageKey = DEFAULT_OPEN_SECTIONS_STORAGE_KEY,
}) => {
	const { pathname } = useLocation();
	const generalActive = GENERAL_ROUTES.some(
		(route) => pathname === route || pathname.startsWith(`${route}/`),
	);
	const { openSections, toggleSection } = useOpenSections(
		openSectionsStorageKey,
		generalActive ? [GENERAL_SECTION] : [],
		[GENERAL_SECTION],
	);
	const isActive = (href: string) =>
		pathname === href || pathname.startsWith(`${href}/`);

	return (
		<div className="flex flex-col gap-3">
			<SidebarAccordion
				icon={UserIcon}
				label="General"
				open={openSections.has(GENERAL_SECTION)}
				onToggle={() => toggleSection(GENERAL_SECTION)}
				active={generalActive}
			>
				{/* Nested leaf rows hang off a connecting line at the label edge. */}
				<div className="flex flex-col gap-1 pl-3 border-0 border-l border-solid border-border">
					<SidebarNavLink nested href="/settings/account">
						Account
					</SidebarNavLink>
					<SidebarNavLink nested href="/settings/appearance">
						Appearance
					</SidebarNavLink>
					<SidebarNavLink nested href="/settings/notifications">
						Notifications
					</SidebarNavLink>
					{showSchedulePage && (
						<SidebarNavLink nested href="/settings/schedule">
							Schedule
						</SidebarNavLink>
					)}
				</div>
			</SidebarAccordion>

			<SidebarTopLevelNavItem
				label="External authentication"
				href="/settings/external-auth"
				icon={UserLockIcon}
				active={isActive("/settings/external-auth")}
			/>
			{showOAuth2Page && (
				<SidebarTopLevelNavItem
					label="OAuth2 applications"
					href="/settings/oauth2-provider"
					icon={BlocksIcon}
					active={isActive("/settings/oauth2-provider")}
				/>
			)}
			<SidebarTopLevelNavItem
				label="Security"
				href="/settings/security"
				icon={ShieldCheckIcon}
				active={isActive("/settings/security")}
			/>
			<SidebarTopLevelNavItem
				label="SSH keys"
				href="/settings/ssh-keys"
				icon={KeyRoundIcon}
				active={isActive("/settings/ssh-keys")}
			/>
			<SidebarTopLevelNavItem
				label="Tokens"
				href="/settings/tokens"
				icon={FingerprintIcon}
				active={isActive("/settings/tokens")}
			/>
			<SidebarTopLevelNavItem
				label="Secrets"
				href="/settings/secrets"
				icon={VenetianMaskIcon}
				active={isActive("/settings/secrets")}
			/>
		</div>
	);
};
