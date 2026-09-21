import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { roles } from "#/api/queries/roles";
import {
	activateUser,
	deleteUser,
	suspendUser,
	updatePassword,
	updateRoles,
	userKey,
} from "#/api/queries/users";
import type { User } from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import { RoleSelectorDialog } from "#/modules/roles/RoleSelectorDialog";
import { generateRandomBase64String } from "#/utils/random";
import { ResetPasswordDialog } from "./ResetPasswordDialog";

export type UserAdminAction =
	| { type: "editRoles"; user: User }
	| { type: "resetPassword"; user: User }
	| { type: "suspend"; user: User }
	| { type: "activate"; user: User }
	| { type: "delete"; user: User };

type UserActionDialogsProps = {
	action: UserAdminAction | undefined;
	onClose: () => void;
};

export const UserActionDialogs: FC<UserActionDialogsProps> = ({
	action,
	onClose,
}) => {
	const queryClient = useQueryClient();
	const user = action?.user;
	const rolesQuery = useQuery({
		...roles(),
		enabled: action?.type === "editRoles",
	});
	const updateUserRolesMutation = useMutation(updateRoles(queryClient));
	const deleteUserMutation = useMutation(deleteUser(queryClient));
	const suspendUserMutation = useMutation(suspendUser(queryClient));
	const activateUserMutation = useMutation(activateUser(queryClient));
	const updatePasswordMutation = useMutation(updatePassword());

	const invalidateUser = (target: User) =>
		Promise.all([
			queryClient.invalidateQueries({ queryKey: userKey(target.id) }),
			queryClient.invalidateQueries({ queryKey: userKey(target.username) }),
		]);

	if (!action || !user) {
		return null;
	}

	return (
		<>
			{action.type === "editRoles" && (
				<RoleSelectorDialog
					user={user}
					availableRoles={rolesQuery.data}
					loading={rolesQuery.isLoading}
					error={rolesQuery.error}
					onCancel={onClose}
					onUpdateRoles={async (nextRoles) => {
						try {
							await updateUserRolesMutation.mutateAsync({
								userId: user.id,
								roles: nextRoles,
							});
							await invalidateUser(user);
							toast.success("User roles updated successfully.");
							onClose();
						} catch (error) {
							toast.error(
								getErrorMessage(error, "Error updating user roles."),
								{
									description: getErrorDetail(error),
								},
							);
						}
					}}
					isUpdatingRoles={updateUserRolesMutation.isPending}
				/>
			)}

			{action.type === "delete" && (
				<DeleteDialog
					isOpen
					confirmLoading={deleteUserMutation.isPending}
					name={user.username}
					entity="user"
					onCancel={onClose}
					onConfirm={async () => {
						try {
							await deleteUserMutation.mutateAsync(user.id);
							onClose();
							toast.success(`User "${user.username}" deleted successfully.`);
						} catch (error) {
							toast.error(
								getErrorMessage(
									error,
									`Error deleting user "${user.username}".`,
								),
								{
									description: getErrorDetail(error),
								},
							);
						}
					}}
				/>
			)}

			{action.type === "suspend" && (
				<ConfirmDialog
					type="delete"
					hideCancel={false}
					open
					confirmLoading={suspendUserMutation.isPending}
					title="Suspend user"
					confirmText="Suspend"
					onClose={onClose}
					onConfirm={async () => {
						try {
							await suspendUserMutation.mutateAsync(user.id);
							await invalidateUser(user);
							onClose();
							toast.success(`User "${user.username}" suspended successfully.`);
						} catch (error) {
							toast.error(
								getErrorMessage(
									error,
									`Error suspending user "${user.username}".`,
								),
								{
									description: getErrorDetail(error),
								},
							);
						}
					}}
					description={
						<>
							Do you want to suspend the user <strong>{user.username}</strong>?
						</>
					}
				/>
			)}

			{action.type === "activate" && (
				<ConfirmDialog
					type="success"
					hideCancel={false}
					open
					confirmLoading={activateUserMutation.isPending}
					title="Activate user"
					confirmText="Activate"
					onClose={onClose}
					onConfirm={async () => {
						try {
							await activateUserMutation.mutateAsync(user.id);
							await invalidateUser(user);
							onClose();
							toast.success(`User "${user.username}" activated successfully.`);
						} catch (error) {
							toast.error(
								getErrorMessage(
									error,
									`Error activating user "${user.username}".`,
								),
								{
									description: getErrorDetail(error),
								},
							);
						}
					}}
					description={
						<>
							Do you want to activate <strong>{user.username}</strong>?
						</>
					}
				/>
			)}

			{action.type === "resetPassword" && (
				<ResetPasswordAction
					user={user}
					loading={updatePasswordMutation.isPending}
					onClose={onClose}
					onConfirm={async (newPassword) => {
						try {
							await updatePasswordMutation.mutateAsync({
								userId: user.id,
								password: newPassword,
								old_password: "",
							});
							onClose();
							toast.success(
								`Password for "${user.username}" updated successfully.`,
							);
						} catch (error) {
							toast.error(
								getErrorMessage(
									error,
									`Error resetting password for "${user.username}".`,
								),
							);
						}
					}}
				/>
			)}
		</>
	);
};

const ResetPasswordAction: FC<{
	user: User;
	loading: boolean;
	onClose: () => void;
	onConfirm: (newPassword: string) => void;
}> = ({ user, loading, onClose, onConfirm }) => {
	const [newPassword] = useState(() =>
		process.env.STORYBOOK === "true"
			? "hello-storybook"
			: generateRandomBase64String(12),
	);

	return (
		<ResetPasswordDialog
			open
			loading={loading}
			user={user}
			newPassword={newPassword}
			onClose={onClose}
			onConfirm={() => onConfirm(newPassword)}
		/>
	);
};
