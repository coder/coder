import Button from "@mui/material/Button";
import { type FC, useEffect, useState } from "react";
import { useQuery } from "react-query";
import { groupsForUser } from "api/queries/groups";
import { DisabledBadge, EnabledBadge } from "components/Badges/Badges";
import { Stack } from "components/Stack/Stack";
import { useAuthContext } from "contexts/auth/AuthProvider";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useDashboard } from "modules/dashboard/useDashboard";
import { Section } from "../Section";
import { AccountForm } from "./AccountForm";
import { AccountUserGroups } from "./AccountUserGroups";

export const AccountPage: FC = () => {
  const { user: me, permissions, organizationId } = useAuthenticated();
  const { updateProfile, updateProfileError, isUpdatingProfile } =
    useAuthContext();
  const { entitlements, experiments } = useDashboard();

  const hasGroupsFeature = entitlements.features.user_role_management.enabled;
  const groupsQuery = useQuery({
    ...groupsForUser(organizationId, me.id),
    enabled: hasGroupsFeature,
  });

  const multiOrgExperimentEnabled = experiments.includes("multi-organization");
  const [multiOrgUiEnabled, setMultiOrgUiEnabled] = useState(
    () =>
      multiOrgExperimentEnabled &&
      Boolean(localStorage.getItem("enableMultiOrganizationUi")),
  );

  useEffect(() => {
    if (multiOrgUiEnabled) {
      localStorage.setItem("enableMultiOrganizationUi", "true");
    } else {
      localStorage.removeItem("enableMultiOrganizationUi");
    }
  }, [multiOrgUiEnabled]);

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

      {multiOrgExperimentEnabled && (
        <Section
          title="Organizations"
          description={
            <span>Danger: enabling will break things in the UI.</span>
          }
        >
          <Stack>
            {multiOrgUiEnabled ? <EnabledBadge /> : <DisabledBadge />}
            <Button onClick={() => setMultiOrgUiEnabled((enabled) => !enabled)}>
              {multiOrgUiEnabled ? "Disable" : "Enable"} frontend
              multi-organization support
            </Button>
          </Stack>
        </Section>
      )}
    </Stack>
  );
};

export default AccountPage;
