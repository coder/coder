import { getErrorMessage } from "api/errors";
import { deploymentConfig } from "api/queries/deployment";
import { groupsByUserId } from "api/queries/groups";
import { roles } from "api/queries/roles";
import {
	activateUser,
	authMethods,
	deleteUser,
	paginatedUsers,
	suspendUser,
	updatePassword,
	updateRoles,
} from "api/queries/users";
import type { User } from "api/typesGenerated";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import { useFilter } from "components/Filter/Filter";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { isNonInitialPage } from "components/PaginationWidget/utils";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { usePaginatedQuery } from "hooks/usePaginatedQuery";
import { useDashboard } from "modules/dashboard/useDashboard";
import { type FC, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useLocation, useNavigate, useSearchParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { generateRandomString } from "utils/random";
import { ResetPasswordDialog } from "./ResetPasswordDialog";
import { useStatusFilterMenu } from "./UsersFilter";
import { UsersPageView } from "./UsersPageView";

type UserPageProps = {
	// Used by Storybook to prevent generating a new password each time the story
	// loads, avoiding Chromatic snapshot differences.
	defaultNewPassword?: string;
};

const UsersPage: FC<UserPageProps> = ({ defaultNewPassword }) => {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const location = useLocation();
	const searchParamsResult = useSearchParams();
	const { entitlements } = useDashboard();
	const [searchParams] = searchParamsResult;

	const groupsByUserIdQuery = useQuery(groupsByUserId());
	const authMethodsQuery = useQuery(authMethods());

	const { permissions, user: me } = useAuthenticated();
	const {
		createUser: canCreateUser,
		updateUsers: canEditUsers,
		viewDeploymentValues,
	} = permissions;
	const rolesQuery = useQuery(roles());
	const { data: deploymentValues } = useQuery({
		...deploymentConfig(),
		enabled: viewDeploymentValues,
	});

	const usersQuery = usePaginatedQuery(paginatedUsers(searchParamsResult[0]));
	const useFilterResult = useFilter({
		searchParamsResult,
		onUpdate: usersQuery.goToFirstPage,
	});

	const statusMenu = useStatusFilterMenu({
		value: useFilterResult.values.status,
		onChange: (option) =>
			useFilterResult.update({
				...useFilterResult.values,
				status: option?.value,
			}),
	});

	const [userToSuspend, setUserToSuspend] = useState<User>();
	const suspendUserMutation = useMutation(suspendUser(queryClient));

	const [userToActivate, setUserToActivate] = useState<User>();
	const activateUserMutation = useMutation(activateUser(queryClient));

	const [userToDelete, setUserToDelete] = useState<User>();
	const deleteUserMutation = useMutation(deleteUser(queryClient));

	const [confirmResetPassword, setConfirmResetPassword] = useState<{
		user: User;
		newPassword: string;
	}>();

	const updatePasswordMutation = useMutation(updatePassword());
	const updateRolesMutation = useMutation(updateRoles(queryClient));

	// Indicates if oidc roles are synced from the oidc idp.
	// Assign 'false' if unknown.
	const oidcRoleSyncEnabled =
		viewDeploymentValues &&
		deploymentValues?.config.oidc?.user_role_field !== "";

	const isLoading =
		usersQuery.isLoading ||
		rolesQuery.isLoading ||
		authMethodsQuery.isLoading ||
		groupsByUserIdQuery.isLoading;

	return (
		<>
			<Helmet>
				<title>{pageTitle("Users")}</title>
			</Helmet>

			<UsersPageView
				oidcRoleSyncEnabled={oidcRoleSyncEnabled}
				roles={rolesQuery.data}
				users={usersQuery.data?.users}
				groupsByUserId={groupsByUserIdQuery.data}
				authMethods={authMethodsQuery.data}
				onListWorkspaces={(user) => {
					navigate(
						`/workspaces?filter=${encodeURIComponent(`owner:${user.username}`)}`,
					);
				}}
				onViewActivity={(user) => {
					navigate(
						`/audit?filter=${encodeURIComponent(`username:${user.username}`)}`,
					);
				}}
				onDeleteUser={setUserToDelete}
				onSuspendUser={setUserToSuspend}
				onActivateUser={setUserToActivate}
				onResetUserPassword={(user) => {
					setConfirmResetPassword({
						user,
						newPassword: defaultNewPassword ?? generateRandomString(12),
					});
				}}
				onUpdateUserRoles={async (userId, roles) => {
					try {
						await updateRolesMutation.mutateAsync({ userId, roles });
						displaySuccess("Successfully updated the user roles.");
					} catch (e) {
						displayError(
							getErrorMessage(e, "Error on updating the user roles."),
						);
					}
				}}
				isUpdatingUserRoles={updateRolesMutation.isLoading}
				isLoading={isLoading}
				canEditUsers={canEditUsers}
				canViewActivity={entitlements.features.audit_log.enabled}
				isNonInitialPage={isNonInitialPage(searchParams)}
				actorID={me.id}
				filterProps={{
					filter: useFilterResult,
					error: usersQuery.error,
					menus: { status: statusMenu },
				}}
				usersQuery={usersQuery}
				canCreateUser={canCreateUser}
			/>

			<DeleteDialog
				key={userToDelete?.username}
				isOpen={userToDelete !== undefined}
				confirmLoading={deleteUserMutation.isLoading}
				name={userToDelete?.username ?? ""}
				entity="user"
				onCancel={() => setUserToDelete(undefined)}
				onConfirm={async () => {
					if (!userToDelete) {
						return;
					}
					try {
						await deleteUserMutation.mutateAsync(userToDelete.id);
						setUserToDelete(undefined);
						displaySuccess("Successfully deleted the user.");
					} catch (e) {
						displayError(getErrorMessage(e, "Error deleting user."));
					}
				}}
			/>

			<ConfirmDialog
				type="delete"
				hideCancel={false}
				open={userToSuspend !== undefined}
				confirmLoading={suspendUserMutation.isLoading}
				title="Suspend user"
				confirmText="Suspend"
				onClose={() => setUserToSuspend(undefined)}
				onConfirm={async () => {
					if (!userToSuspend) {
						return;
					}
					try {
						await suspendUserMutation.mutateAsync(userToSuspend.id);
						setUserToSuspend(undefined);
						displaySuccess("Successfully suspended the user.");
					} catch (e) {
						displayError(getErrorMessage(e, "Error suspending user."));
					}
				}}
				description={
					<>
						Do you want to suspend the user{" "}
						<strong>{userToSuspend?.username ?? ""}</strong>?
					</>
				}
			/>

			<ConfirmDialog
				type="success"
				hideCancel={false}
				open={userToActivate !== undefined}
				confirmLoading={activateUserMutation.isLoading}
				title="Activate user"
				confirmText="Activate"
				onClose={() => setUserToActivate(undefined)}
				onConfirm={async () => {
					if (!userToActivate) {
						return;
					}
					try {
						await activateUserMutation.mutateAsync(userToActivate.id);
						setUserToActivate(undefined);
						displaySuccess("Successfully activated the user.");
					} catch (e) {
						displayError(getErrorMessage(e, "Error activating user."));
					}
				}}
				description={
					<>
						Do you want to activate{" "}
						<strong>{userToActivate?.username ?? ""}</strong>?
					</>
				}
			/>

			<ResetPasswordDialog
				key={confirmResetPassword?.user.username}
				open={confirmResetPassword !== undefined}
				loading={updatePasswordMutation.isLoading}
				user={confirmResetPassword?.user}
				newPassword={confirmResetPassword?.newPassword}
				onClose={() => {
					setConfirmResetPassword(undefined);
				}}
				onConfirm={async () => {
					if (!confirmResetPassword) {
						return;
					}
					try {
						await updatePasswordMutation.mutateAsync({
							userId: confirmResetPassword.user.id,
							password: confirmResetPassword.newPassword,
							old_password: "",
						});
						setConfirmResetPassword(undefined);
						displaySuccess("Successfully updated the user password.");
					} catch (e) {
						displayError(
							getErrorMessage(e, "Error on resetting the user password."),
						);
					}
				}}
			/>
		</>
	);
};

export default UsersPage;
