import type { FC } from "react";
import { Avatar } from "#/components/Avatar/Avatar";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { UserDropdownContent } from "#/modules/dashboard/Navbar/UserDropdown/UserDropdownContent";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { UsageIndicator } from "../../UsageIndicator";

export const UserSidebarFooter: FC = () => {
	const { user, signOut } = useAuthenticated();
	const { appearance, buildInfo } = useDashboard();

	return (
		<div className="hidden border-0 border-t border-solid sm:block">
			{/* This footer is resizable, so child sizing must follow its container width instead of the viewport. */}
			<div className="flex min-w-0 items-stretch [container-type:inline-size]">
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<button
							type="button"
							aria-label={`Account menu for ${user.name || user.username}`}
							className="flex min-w-0 flex-1 items-center gap-2 bg-transparent border-0 cursor-pointer px-3 py-3 text-left hover:bg-surface-tertiary/50 transition-colors"
						>
							<Avatar
								fallback={user.username}
								src={user.avatar_url}
								size="sm"
							/>
							<span className="min-w-0 flex-1 truncate text-sm text-content-secondary">
								{user.name || user.username}
							</span>
						</button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="start" className="min-w-auto w-[260px]">
						<UserDropdownContent
							user={user}
							buildInfo={buildInfo}
							supportLinks={
								appearance.support_links?.filter(
									(link) => link.location !== "navbar",
								) ?? []
							}
							onSignOut={signOut}
						/>
					</DropdownMenuContent>
				</DropdownMenu>
				<UsageIndicator />
			</div>
		</div>
	);
};
