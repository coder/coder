import { type FC } from "react";
import { useMe } from "hooks/useMe";
import { usePermissions } from "hooks/usePermissions";
import { useQuery } from "react-query";
import { groupsForUser } from "api/queries/groups";
import { useOrganizationId } from "hooks";
import { useAuth } from "components/AuthProvider/AuthProvider";

import { Stack } from "@mui/system";
import { AccountUserGroups } from "./AccountUserGroups";
import { AccountForm } from "./AccountForm";
import { Section } from "components/SettingsLayout/Section";

export const AccountPage: FC = () => {
  const { updateProfile, updateProfileError, isUpdatingProfile } = useAuth();
  const permissions = usePermissions();

  const me = useMe();
  const organizationId = useOrganizationId();
  const groupsQuery = useQuery(groupsForUser(organizationId, me.id));

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

      {/* Has <Section> embedded inside because its description is dynamic */}
      <AccountUserGroups
        groups={groupsQuery.data}
        loading={groupsQuery.isLoading}
        error={groupsQuery.error}
      />
    </Stack>
  );
};

export default AccountPage;
