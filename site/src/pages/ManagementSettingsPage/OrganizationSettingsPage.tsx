import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Navigate, useNavigate, useParams } from "react-router-dom";
import {
  updateOrganization,
  deleteOrganization,
  organizationsPermissions,
} from "api/queries/organizations";
import type { Organization } from "api/typesGenerated";
import { EmptyState } from "components/EmptyState/EmptyState";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import {
  canEditOrganization,
  useOrganizationSettings,
} from "./ManagementSettingsLayout";
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

  const organization =
    organizations && organizationName
      ? getOrganizationByName(organizations, organizationName)
      : undefined;
  const permissionsQuery = useQuery(
    organizationsPermissions(organizations?.map((o) => o.id)),
  );

  const permissions = permissionsQuery.data;
  if (!organizations || !permissions) {
    return <Loader />;
  }

  // Redirect /organizations => /organizations/default-org, or if they cannot edit
  // the default org, then the first org they can edit, if any.
  if (!organizationName) {
    const editableOrg = organizations
      .sort((a, b) => {
        // Prefer default org (it may not be first).
        // JavaScript will happily subtract booleans, but use numbers to keep
        // the compiler happy.
        return (b.is_default ? 1 : 0) - (a.is_default ? 1 : 0);
      })
      .find((org) => canEditOrganization(permissions[org.id]));
    if (editableOrg) {
      return <Navigate to={`/organizations/${editableOrg.name}`} replace />;
    }
    return <EmptyState message="No organizations found" />;
  }

  if (!organization) {
    return <EmptyState message="Organization not found" />;
  }

  const error =
    updateOrganizationMutation.error ?? deleteOrganizationMutation.error;

  return (
    <OrganizationSettingsPageView
      canEdit={permissions[organization.id]?.editOrganization ?? false}
      organization={organization}
      error={error}
      onSubmit={async (values) => {
        const updatedOrganization =
          await updateOrganizationMutation.mutateAsync({
            organizationId: organization.id,
            req: values,
          });
        navigate(`/organizations/${updatedOrganization.name}`);
        displaySuccess("Organization settings updated.");
      }}
      onDeleteOrganization={() => {
        deleteOrganizationMutation.mutate(organization.id);
        displaySuccess("Organization deleted.");
        navigate("/organizations");
      }}
    />
  );
};

export default OrganizationSettingsPage;

const getOrganizationByName = (organizations: Organization[], name: string) =>
  organizations.find((org) => org.name === name);
