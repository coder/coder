import type { FC } from "react";
import { useQuery } from "react-query";
import { groupsForUser } from "#/api/queries/groups";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { useAuthContext } from "#/contexts/auth/AuthProvider";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { AccountForm } from "./AccountForm";
import { AccountUserGroups } from "./AccountUserGroups";

const AccountPage: FC = () => {
	const { permissions, user: me } = useAuthenticated();
	const { updateProfile, updateProfileError, isUpdatingProfile } =
		useAuthContext();
	const { entitlements } = useDashboard();

	const hasGroupsFeature = entitlements.features.user_role_management.enabled;
	const groupsQuery = useQuery({
		...groupsForUser(me.id),
		enabled: hasGroupsFeature,
	});

	return (
		<div className="flex flex-col gap-12">
			<div>
				<SettingsHeader>
					<SettingsHeaderTitle>Account</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Update your account info.
					</SettingsHeaderDescription>
				</SettingsHeader>
				<AccountForm
					editable={permissions?.updateUsers ?? false}
					email={me.email}
					updateProfileError={updateProfileError}
					isLoading={isUpdatingProfile}
					initialValues={{ username: me.username, name: me.name ?? "" }}
					onSubmit={updateProfile}
				/>
			</div>

			{hasGroupsFeature && (
				<AccountUserGroups
					groups={groupsQuery.data}
					loading={groupsQuery.isLoading}
					error={groupsQuery.error}
				/>
			)}
		</div>
	);
};

export default AccountPage;
