import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useSearchParams } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { deploymentConfig } from "#/api/queries/deployment";
import { groupsByUserId } from "#/api/queries/groups";
import { roles } from "#/api/queries/roles";
import {
	activateUser,
	deleteUser,
	paginatedUsers,
	suspendUser,
	updatePassword,
	updateRoles,
} from "#/api/queries/users";
import type { User } from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import { DeleteDialog } from "#/components/Dialogs/DeleteDialog/DeleteDialog";
import { useFilter } from "#/components/Filter/Filter";
import { useStatusFilterMenu } from "#/components/Filter/UsersFilter";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { usePaginatedQuery } from "#/hooks/usePaginatedQuery";
import { shouldShowAISeatColumn } from "#/modules/dashboard/entitlements";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { RoleSelectorDialog } from "#/modules/roles/RoleSelectorDialog";
import { pageTitle } from "#/utils/page";
import { generateRandomString } from "#/utils/random";
import { ResetPasswordDialog } from "./ResetPasswordDialog";
import { UsersPageView } from "./UsersPageView";

const UsersPage: React.FC = () => {
	const queryClient = useQueryClient();
	const [searchParams, setSearchParams] = useSearchParams();
	const { entitlements } = useDashboard();
	const showAISeatColumn = shouldShowAISeatColumn(entitlements);

	const groupsByUserIdQuery = useQuery(groupsByUserId());

	const { permissions, user: me } = useAuthenticated();
	const {
		createUser: canCreateUser,
		updateUsers: canEditUsers,
		viewDeploymentConfig,
	} = permissions;
	const rolesQuery = useQuery(roles());
	const { data: deploymentValues } = useQuery({
		...deploymentConfig(),
		enabled: viewDeploymentConfig,
	});

	const usersQuery = usePaginatedQuery(paginatedUsers(searchParams));
	const useFilterResult = useFilter({
		searchParams,
		onSearchParamsChange: setSearchParams,
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

	const [userToSuspend, setUserToSuspend] = useState<User | undefined>(
		undefined,
	);
	const suspendUserMutation = useMutation(suspendUser(queryClient));

	const [userToActivate, setUserToActivate] = useState<User | undefined>(
		undefined,
	);
	const activateUserMutation = useMutation(activateUser(queryClient));

	const [userToDelete, setUserToDelete] = useState<User | undefined>(undefined);
	const deleteUserMutation = useMutation(deleteUser(queryClient));

	const [userToEditRoles, setUserToEditRoles] = useState<User | undefined>(
		undefined,
	);
	const updateUserRolesMutation = useMutation(updateRoles(queryClient));

	const [confirmResetPassword, setConfirmResetPassword] = useState<{
		user: User;
		newPassword: string;
	}>();
	const updatePasswordMutation = useMutation(updatePassword());

	// Indicates if oidc roles are synced from the oidc idp.
	// Assign 'false' if unknown.
	const oidcRoleSyncEnabled =
		viewDeploymentConfig &&
		deploymentValues?.config.oidc?.user_role_field !== "";

	const isLoading =
		usersQuery.isLoading ||
		rolesQuery.isLoading ||
		groupsByUserIdQuery.isLoading;

	return (
		<>
			<title>{pageTitle("Users")}</title>

			<UsersPageView
				isLoading={isLoading}
				filterProps={{
					filter: useFilterResult,
					error: usersQuery.error,
					menus: { status: statusMenu },
				}}
				usersQuery={usersQuery}
				groupsByUserId={groupsByUserIdQuery.data}
				showAISeatColumn={showAISeatColumn}
				onEditUserRoles={setUserToEditRoles}
				isUpdatingUserRoles={updateUserRolesMutation.isPending}
				onResetUserPassword={(user) => {
					setConfirmResetPassword({
						user,
						newPassword:
							process.env.STORYBOOK === "true"
								? "hello-storybook"
								: generateRandomString(12),
					});
				}}
				onSuspendUser={setUserToSuspend}
				onActivateUser={setUserToActivate}
				onDeleteUser={setUserToDelete}
				me={me.id}
				canCreateUser={canCreateUser}
				canEditUsers={canEditUsers}
				canViewActivity={entitlements.features.audit_log.enabled}
				oidcRoleSyncEnabled={oidcRoleSyncEnabled}
			/>

			<RoleSelectorDialog
				key={userToEditRoles?.username}
				user={userToEditRoles}
				availableRoles={rolesQuery.data}
				onCancel={() => setUserToEditRoles(undefined)}
				onUpdateRoles={async (roles) => {
					try {
						await updateUserRolesMutation.mutateAsync({
							userId: userToEditRoles!.id,
							roles,
						});
						toast.success("User roles updated successfully.");
						setUserToEditRoles(undefined);
					} catch (e) {
						toast.error(getErrorMessage(e, "Error updating user roles."), {
							description: getErrorDetail(e),
						});
					}
				}}
				isUpdatingRoles={updateUserRolesMutation.isPending}
			/>

			<DeleteDialog
				key={userToDelete?.username}
				isOpen={userToDelete !== undefined}
				confirmLoading={deleteUserMutation.isPending}
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
						toast.success(
							`User "${userToDelete.username}" deleted successfully.`,
						);
					} catch (e) {
						toast.error(
							getErrorMessage(
								e,
								`Error deleting user "${userToDelete.username}".`,
							),
							{
								description: getErrorDetail(e),
							},
						);
					}
				}}
			/>

			<ConfirmDialog
				type="delete"
				hideCancel={false}
				open={userToSuspend !== undefined}
				confirmLoading={suspendUserMutation.isPending}
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
						toast.success(
							`User "${userToSuspend.username}" suspended successfully.`,
						);
					} catch (e) {
						toast.error(
							getErrorMessage(
								e,
								`Error suspending user "${userToSuspend.username}".`,
							),
							{
								description: getErrorDetail(e),
							},
						);
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
				confirmLoading={activateUserMutation.isPending}
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
						toast.success(
							`User "${userToActivate.username}" activated successfully.`,
						);
					} catch (e) {
						toast.error(
							getErrorMessage(
								e,
								`Error activating user "${userToActivate.username}".`,
							),
							{
								description: getErrorDetail(e),
							},
						);
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
				loading={updatePasswordMutation.isPending}
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
						toast.success(
							`Password for "${confirmResetPassword.user.username}" updated successfully.`,
						);
					} catch (e) {
						toast.error(
							getErrorMessage(
								e,
								`Error resetting password for "${confirmResetPassword.user.username}".`,
							),
						);
					}
				}}
			/>
		</>
	);
};

export default UsersPage;
