import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router-dom";
import {
  updateOrganization,
  deleteOrganization,
} from "api/queries/organizations";
import type { Organization } from "api/typesGenerated";
import { EmptyState } from "components/EmptyState/EmptyState";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { useOrganizationSettings } from "./ManagementSettingsLayout";
import { OrganizationSettingsPageView } from "./OrganizationSettingsPageView";

const OrganizationSettingsPage: FC = () => {
  const { organization: organizationName } = useParams() as {
    organization?: string;
  };
  const { organizations } = useOrganizationSettings();

  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const updateOrganizationMutation = useMutation(
    updateOrganization(queryClient),
  );
  const deleteOrganizationMutation = useMutation(
    deleteOrganization(queryClient),
  );

  const org = organizationName
    ? getOrganizationByName(organizations, organizationName)
    : getOrganizationByDefault(organizations);

  const error =
    updateOrganizationMutation.error ?? deleteOrganizationMutation.error;

  if (!org) {
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

const getOrganizationByDefault = (organizations: Organization[]) =>
  organizations.find((org) => org.is_default);

const getOrganizationByName = (organizations: Organization[], name: string) =>
  organizations.find((org) => org.name === name);
