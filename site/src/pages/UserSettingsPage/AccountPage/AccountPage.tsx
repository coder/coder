import { type FC } from "react";
import { useQuery } from "react-query";
import { groupsForUser } from "api/queries/groups";
import { useAuth } from "contexts/auth/useAuth";
import { useMe } from "contexts/auth/useMe";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { usePermissions } from "contexts/auth/usePermissions";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { Stack } from "components/Stack/Stack";
import { Section } from "../Section";
import { AccountUserGroups } from "./AccountUserGroups";
import { AccountForm } from "./AccountForm";

export const AccountPage: FC = () => {
  const me = useMe();
  const permissions = usePermissions();
  const organizationId = useOrganizationId();
  const { updateProfile, updateProfileError, isUpdatingProfile } = useAuth();
  const { entitlements } = useDashboard();

  const hasGroupsFeature = entitlements.features.user_role_management.enabled;
  const groupsQuery = useQuery({
    ...groupsForUser(organizationId, me.id),
    enabled: hasGroupsFeature,
  });

  return (
    <Stack spacing={6}>
      <Section title="Account" description="Update your account info">
        <AccountForm
          editable={permissions?.updateUsers ?? false}
          email={me.email}
          updateProfileError={updateProfileError}
          isLoading={isUpdatingProfile}
          initialValues={{ username: me.username, name: me.name }}
          onSubmit={updateProfile}
        />
      </Section>

      {hasGroupsFeature && (
        <AccountUserGroups
          groups={groupsQuery.data}
          loading={groupsQuery.isLoading}
          error={groupsQuery.error}
        />
      )}
    </Stack>
  );
};

export default AccountPage;
