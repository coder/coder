import { Stack } from "@mui/system";
import { type FC } from "react";
import { useQuery } from "react-query";
import { useOrganizationId } from "hooks";
import { useMe } from "hooks/useMe";
import { usePermissions } from "hooks/usePermissions";
import { groupsForUser } from "api/queries/groups";
import { useAuth } from "contexts/AuthProvider/AuthProvider";
import { useDashboard } from "components/Dashboard/DashboardProvider";
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
          initialValues={{ username: me.username }}
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
