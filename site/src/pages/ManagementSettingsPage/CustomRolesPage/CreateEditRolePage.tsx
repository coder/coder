import Button from "@mui/material/Button";
import { useFormik } from "formik";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router-dom";
import * as Yup from "yup";
import { getErrorMessage } from "api/errors";
import { organizationPermissions } from "api/queries/organizations";
import { patchOrganizationRole, organizationRoles } from "api/queries/roles";
import type { PatchRoleRequest } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import { nameValidator } from "utils/formUtils";
import { pageTitle } from "utils/page";
import { useOrganizationSettings } from "../ManagementSettingsLayout";
import CreateEditRolePageView from "./CreateEditRolePageView";

export const CreateEditRolePage: FC = () => {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { organization: organizationName, roleName } = useParams() as {
    organization: string;
    roleName: string;
  };
  const { organizations } = useOrganizationSettings();
  const organization = organizations?.find((o) => o.name === organizationName);
  const permissionsQuery = useQuery(organizationPermissions(organization?.id));
  const patchOrganizationRoleMutation = useMutation(
    patchOrganizationRole(queryClient, organizationName),
  );
  const { data: roleData, isLoading } = useQuery(
    organizationRoles(organizationName),
  );
  const role = roleData?.find((role) => role.name === roleName);
  const permissions = permissionsQuery.data;

  const validationSchema = Yup.object({
    name: nameValidator("Name"),
  });

  const onSubmit = async (data: PatchRoleRequest) => {
    try {
      await patchOrganizationRoleMutation.mutateAsync(data);
      navigate(`/organizations/${organizationName}/roles`);
    } catch (error) {
      displayError(getErrorMessage(error, "Failed to update custom role"));
    }
  };

  const form = useFormik<PatchRoleRequest>({
    initialValues: {
      name: role?.name || "",
      display_name: role?.display_name || "",
      site_permissions: role?.site_permissions || [],
      organization_permissions: role?.organization_permissions || [],
      user_permissions: role?.user_permissions || [],
    },
    validationSchema,
    onSubmit,
  });

  if (isLoading) {
    return <Loader />;
  }

  return (
    <>
      <Helmet>
        <title>
          {pageTitle(
            role !== undefined ? "Edit Custom Role" : "Create Custom Role",
          )}
        </title>
      </Helmet>

      <PageHeader
        actions={
          permissions &&
          permissions.assignOrgRole && (
            <>
              <Button
                onClick={() => {
                  navigate(`/organizations/${organizationName}/roles`);
                }}
              >
                Cancel
              </Button>
              <Button
                variant="contained"
                color="primary"
                onClick={() => {
                  form.handleSubmit();
                }}
              >
                {role !== undefined ? "Save" : "Create Role"}
              </Button>
            </>
          )
        }
      >
        <PageHeaderTitle>
          {role ? "Edit" : "Create"} custom role
        </PageHeaderTitle>
      </PageHeader>

      <CreateEditRolePageView
        role={role}
        form={form}
        error={patchOrganizationRoleMutation.error}
        isLoading={patchOrganizationRoleMutation.isLoading}
      />
    </>
  );
};

export default CreateEditRolePage;
