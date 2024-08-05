import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Navigate, useNavigate, useParams } from "react-router-dom";
import {
  updateOrganization,
  deleteOrganization,
  organizationPermissions,
} from "api/queries/organizations";
import type { Organization } from "api/typesGenerated";
import { EmptyState } from "components/EmptyState/EmptyState";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { linkToAuditing, withFilter } from "modules/navigation";
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

  const organization =
    organizations && organizationName
      ? getOrganizationByName(organizations, organizationName)
      : undefined;
  const permissionsQuery = useQuery(organizationPermissions(organization?.id));

  if (!organizations) {
    return <Loader />;
  }

  // Redirect /organizations => /organizations/default-org
  if (!organizationName) {
    const defaultOrg = getOrganizationByDefault(organizations);
    if (defaultOrg) {
      return <Navigate to={`/organizations/${defaultOrg.name}`} replace />;
    }
    // We expect there to always be a default organization.
    throw new Error("No default organization found");
  }

  if (!organization) {
    return <EmptyState message="Organization not found" />;
  }

  const permissions = permissionsQuery.data;
  if (!permissions) {
    return <Loader />;
  }

  // When someone views the top-level org URL (/organizations/my-org) they might
  // not have edit permissions.  Redirect to a page they can view.
  // TODO: Instead of redirecting, maybe there should be some summary page for
  //       the organization that anyone who belongs to the org can read (with
  //       the description, icon, etc).  Or we could show the form that normally
  //       shows on this page but disable the fields, although that could be
  //       confusing?
  if (!permissions.editOrganization) {
    if (permissions.viewMembers) {
      return <Navigate to="members" replace />;
    } else if (permissions.viewGrousp) {
      return <Navigate to="groups" replace />;
    } else if (permissions.auditOrganization) {
      return (
        <Navigate
          to={`/deployment${withFilter(
            linkToAuditing,
            `organization:${organization.name}`,
          )}`}
          replace
        />
      );
    }
    return (
      <EmptyState message="You do not have permission to edit this organization." />
    );
  }

  const error =
    updateOrganizationMutation.error ?? deleteOrganizationMutation.error;

  return (
    <OrganizationSettingsPageView
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

const getOrganizationByDefault = (organizations: Organization[]) =>
  organizations.find((org) => org.is_default);

const getOrganizationByName = (organizations: Organization[], name: string) =>
  organizations.find((org) => org.name === name);
