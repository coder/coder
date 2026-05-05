import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { roles } from "#/api/queries/roles";
import { authMethods, createUser } from "#/api/queries/users";
import { Margins } from "#/components/Margins/Margins";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { pageTitle } from "#/utils/page";
import { CreateUserForm } from "./CreateUserForm";

const CreateUserPage: FC = () => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const createUserMutation = useMutation(createUser(queryClient));
	const authMethodsQuery = useQuery(authMethods());
	const rolesQuery = useQuery(roles());
	const { showOrganizations } = useDashboard();
	const { service_accounts: serviceAccountsEnabled } = useFeatureVisibility();

	return (
		<Margins>
			<title>{pageTitle("Create User")}</title>

			<CreateUserForm
				error={createUserMutation.error}
				isLoading={createUserMutation.isPending}
				onSubmit={async (user) => {
					const mutation = createUserMutation.mutateAsync(
						{
							username: user.username,
							name: user.name,
							email: user.email,
							organization_ids: [user.organization],
							login_type: user.login_type,
							password: user.password,
							user_status: null,
							service_account: user.service_account,
							roles: [...user.roles],
						},
						{
							onSuccess: () => {
								navigate("..", { relative: "path" });
							},
						},
					);
					toast.promise(mutation, {
						loading: `Creating user "${user.username}"...`,
						success: `User "${user.username}" created successfully.`,
						error: (e) => ({
							message: getErrorMessage(
								e,
								`Failed to create user "${user.username}".`,
							),
							description: getErrorDetail(e),
						}),
					});
				}}
				onCancel={() => {
					navigate("..", { relative: "path" });
				}}
				authMethods={authMethodsQuery.data}
				showOrganizations={showOrganizations}
				serviceAccountsEnabled={serviceAccountsEnabled}
				availableRoles={rolesQuery.data}
				rolesLoading={rolesQuery.isLoading}
				rolesError={rolesQuery.error}
			/>
		</Margins>
	);
};

export default CreateUserPage;
