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
import { AUDIT_LINK, withFilter } from "modules/navigation";
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

  // TODO: If we could query permissions based on the name then we would not
  //       have to cascade off the organizations query.
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
    return <EmptyState message="No default organization found" />;
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
            AUDIT_LINK,
            `organization:${organization.name}`,
          )}`}
          replace
        />
      );
    }
    // TODO: This should only happen if the user manually edits the URL, but
    //       maybe we can show a better message anyway.
    return (
      <EmptyState message={organization.display_name || organization.name} />
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
