import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router-dom";
import {
  updateOrganization,
  deleteOrganization,
} from "api/queries/organizations";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Stack } from "components/Stack/Stack";
import { useOrganizationSettings } from "./ManagementSettingsLayout";
import { OrganizationSettingsPageView } from "./OrganizationSettingsPageView";

const OrganizationSettingsPage: FC = () => {
  const navigate = useNavigate();

  const queryClient = useQueryClient();
  const updateOrganizationMutation = useMutation(
    updateOrganization(queryClient),
  );
  const deleteOrganizationMutation = useMutation(
    deleteOrganization(queryClient),
  );

  const { currentOrganizationId, organizations } = useOrganizationSettings();

  const org = organizations.find((org) => org.id === currentOrganizationId);

  const error =
    updateOrganizationMutation.error ?? deleteOrganizationMutation.error;

  if (!currentOrganizationId) {
    return null;
  }

  if (!org) {
    return null;
  }

  return (
    <Stack>
      {Boolean(error) && <ErrorAlert error={error} />}

      <OrganizationSettingsPageView
        organization={org}
        error={error}
        onSubmit={async (values) => {
          await updateOrganizationMutation.mutateAsync({
            orgId: org.id,
            req: values,
          });
          displaySuccess("Organization settings updated.");
        }}
        onDeleteOrganization={() => {
          deleteOrganizationMutation.mutate(org.id);
          navigate("/organizations");
        }}
      />
    </Stack>
  );
};

export default OrganizationSettingsPage;
