import type { FC } from "react";
import { Navigate, Outlet } from "react-router";
import { Avatar } from "#/components/Avatar/Avatar";
import { SettingsNavigation } from "#/components/SettingsNavigation/SettingsNavigation";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { pageTitle } from "#/utils/page";
import { userSettingsNavigation } from "./navigation";

export const UserSettingsIndexRedirect: FC = () => (
	<Navigate to="account" replace />
);

const Layout: FC = () => {
	const { user: me } = useAuthenticated();
	const { entitlements, buildInfo } = useDashboard();
	const sections = userSettingsNavigation({
		showSchedulePage:
			entitlements.features.advanced_template_scheduling.enabled,
		showOAuth2Page: buildInfo.oauth2_provider,
	});

	return (
		<>
			<title>{pageTitle("Settings")}</title>
			<SettingsNavigation
				title="User settings"
				sections={sections}
				storageKey="user-settings-nav-collapsed"
				pageAdornment={
					<Avatar size="sm" fallback={me.username} src={me.avatar_url} />
				}
			>
				<Outlet />
			</SettingsNavigation>
		</>
	);
};

export default Layout;
