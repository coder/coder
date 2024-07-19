import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router-dom";
import {
  updateOrganization,
  deleteOrganization,
} from "api/queries/organizations";
import { EmptyState } from "components/EmptyState/EmptyState";
import { displaySuccess } from "components/GlobalSnackbar/utils";
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

  if (!currentOrganizationId || !org) {
    return <EmptyState message="Organization not found" />;
  }

  return (
    <OrganizationSettingsPageView
      organization={org}
      error={error}
      onSubmit={async (values) => {
        const updatedOrganization =
          await updateOrganizationMutation.mutateAsync({
            organizationId: org.id,
            req: values,
          });
        navigate(`/organizations/${updatedOrganization.name}`);
        displaySuccess("Organization settings updated.");
      }}
      onDeleteOrganization={() => {
        deleteOrganizationMutation.mutate(org.id);
        displaySuccess("Organization deleted.");
        navigate("/organizations");
      }}
    />
  );
};

export default OrganizationSettingsPage;
