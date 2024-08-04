import Button from "@mui/material/Button";
import { useFormik } from "formik";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router-dom";
import * as Yup from "yup";
import { getErrorMessage } from "api/errors";
import { patchOrganizationRole, organizationRoles } from "api/queries/roles";
import type { PatchRoleRequest } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { PageHeader } from "components/PageHeader/PageHeader";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { nameValidator } from "utils/formUtils";
import { pageTitle } from "utils/page";
import CreateEditRolePageView from "./CreateEditRolePageView";

export const CreateEditRolePage: FC = () => {
  const { permissions } = useAuthenticated();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { organization, roleName } = useParams() as {
    organization: string;
    roleName: string;
  };
  const { assignOrgRole: canAssignOrgRole } = permissions;
  const patchOrganizationRoleMutation = useMutation(
    patchOrganizationRole(queryClient, organization ?? "default"),
  );
  const { data: roleData, isLoading } = useQuery(
    organizationRoles(organization),
  );
  const role = roleData?.find((role) => role.name === roleName);

  const validationSchema = Yup.object({
    name: nameValidator("Name"),
  });

  const onSubmit = async (data: PatchRoleRequest) => {
    try {
      await patchOrganizationRoleMutation.mutateAsync(data);
      navigate(`/organizations/${organization}/roles`);
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
          canAssignOrgRole && (
            <>
              <Button
                onClick={() => {
                  navigate(`/organizations/${organization}/roles`);
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
      ></PageHeader>

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
