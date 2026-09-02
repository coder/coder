import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { roles } from "#/api/queries/roles";
import { authMethods, createUser } from "#/api/queries/users";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
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
		<>
			<title>{pageTitle("New user")}</title>

			{!authMethodsQuery.data ? (
				authMethodsQuery.error ? (
					<ErrorAlert error={authMethodsQuery.error} />
				) : (
					<Loader />
				)
			) : (
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
						const requestedAccount = user.service_account
							? "service account"
							: "user";
						toast.promise(mutation, {
							loading: `Creating ${requestedAccount} "${user.username}"...`,
							success: (created) =>
								`${created.is_service_account ? "Service account" : "User"} "${created.username}" created successfully.`,
							error: (e) => ({
								message: getErrorMessage(
									e,
									`Failed to create ${requestedAccount} "${user.username}".`,
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
			)}
		</>
	);
};

export default CreateUserPage;
