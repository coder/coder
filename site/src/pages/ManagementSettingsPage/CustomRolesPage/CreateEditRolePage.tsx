import { getErrorMessage } from "api/errors";
import { organizationPermissions } from "api/queries/organizations";
import {
	createOrganizationRole,
	organizationRoles,
	updateOrganizationRole,
} from "api/queries/roles";
import type { CustomRoleRequest } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import CreateEditRolePageView from "./CreateEditRolePageView";
import { useDashboard } from "modules/dashboard/useDashboard";

export const CreateEditRolePage: FC = () => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const { activeOrganization } = useDashboard();
	const { organization: organizationName, roleName } = useParams() as {
		organization: string;
		roleName: string;
	};

	const permissionsQuery = useQuery(
		organizationPermissions(activeOrganization?.id),
	);
	const createOrganizationRoleMutation = useMutation(
		createOrganizationRole(queryClient, organizationName),
	);
	const updateOrganizationRoleMutation = useMutation(
		updateOrganizationRole(queryClient, organizationName),
	);
	const { data: roleData, isLoading } = useQuery(
		organizationRoles(organizationName),
	);
	const role = roleData?.find((role) => role.name === roleName);
	const permissions = permissionsQuery.data;

	if (isLoading || !permissions) {
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

			<CreateEditRolePageView
				role={role}
				onSubmit={async (data: CustomRoleRequest) => {
					try {
						if (role) {
							await updateOrganizationRoleMutation.mutateAsync(data);
						} else {
							await createOrganizationRoleMutation.mutateAsync(data);
						}
						navigate(`/organizations/${organizationName}/roles`);
					} catch (error) {
						displayError(
							getErrorMessage(error, "Failed to update custom role"),
						);
					}
				}}
				error={
					role
						? updateOrganizationRoleMutation.error
						: createOrganizationRoleMutation.error
				}
				isLoading={
					role
						? updateOrganizationRoleMutation.isLoading
						: createOrganizationRoleMutation.isLoading
				}
				organizationName={organizationName}
				canAssignOrgRole={permissions.assignOrgRole}
			/>
		</>
	);
};

export default CreateEditRolePage;
