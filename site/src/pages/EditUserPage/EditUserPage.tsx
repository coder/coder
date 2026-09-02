import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { deploymentConfig } from "#/api/queries/deployment";
import {
	updateProfile,
	user,
	userKey,
	usersQueryKey,
} from "#/api/queries/users";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import {
	UserActionDialogs,
	type UserAdminAction,
} from "#/modules/users/UserActionDialogs";
import { UserMoreActions } from "#/modules/users/UserMoreActions";
import { pageTitle } from "#/utils/page";
import { isUUID } from "#/utils/uuid";
import { EditUserForm } from "./EditUserForm";

const EditUserPage: FC = () => {
	const { user: usernameOrId } = useParams() as { user: string };
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const { user: me, permissions } = useAuthenticated();
	const { entitlements } = useDashboard();
	const userQuery = useQuery(user(usernameOrId));
	const updateProfileMutation = useMutation(
		updateProfile(userQuery.data?.id ?? ""),
	);
	const { data: deploymentValues } = useQuery({
		...deploymentConfig(),
		enabled: permissions.viewDeploymentConfig,
	});
	const oidcRoleSyncEnabled =
		permissions.viewDeploymentConfig &&
		deploymentValues?.config.oidc?.user_role_field !== "";
	const [action, setAction] = useState<UserAdminAction>();

	return (
		<>
			<title>
				{pageTitle(
					userQuery.data
						? `Edit ${userQuery.data.name?.trim() || userQuery.data.username}`
						: "Edit user",
				)}
			</title>

			{userQuery.isLoading ? (
				<Loader />
			) : !userQuery.data ? (
				<ErrorAlert error={userQuery.error} />
			) : (
				<EditUserForm
					error={updateProfileMutation.error}
					isLoading={updateProfileMutation.isPending}
					initialValues={{
						username: userQuery.data.username,
						name: userQuery.data.name ?? "",
						avatar_url: userQuery.data.avatar_url ?? "",
					}}
					canEditAvatar={
						userQuery.data.login_type === "password" ||
						userQuery.data.login_type === "none"
					}
					headerActions={
						permissions.updateUsers && (
							<UserMoreActions
								user={userQuery.data}
								me={me.id}
								showEdit={false}
								canViewActivity={entitlements.features.audit_log.enabled}
								oidcRoleSyncEnabled={oidcRoleSyncEnabled}
								onAction={setAction}
							/>
						)
					}
					onSubmit={(values) => {
						const mutation = updateProfileMutation.mutateAsync(values, {
							onSuccess: (updatedUser) => {
								void queryClient.invalidateQueries({
									queryKey: userKey(usernameOrId),
								});
								void queryClient.invalidateQueries({
									queryKey: usersQueryKey,
								});
								if (
									!isUUID(usernameOrId) &&
									updatedUser.username !== usernameOrId
								) {
									navigate(`../${updatedUser.username}`, {
										relative: "path",
										replace: true,
									});
								}
							},
						});
						toast.promise(mutation, {
							loading: `Saving user "${values.username}"...`,
							success: `User "${values.username}" updated successfully.`,
							error: (error) => ({
								message: getErrorMessage(
									error,
									`Failed to update user "${values.username}".`,
								),
								description: getErrorDetail(error),
							}),
						});
					}}
					onCancel={() => {
						navigate("..", { relative: "path" });
					}}
				/>
			)}

			<UserActionDialogs
				action={action}
				onClose={() => setAction(undefined)}
				onDeleted={() => {
					navigate("..", { relative: "path" });
				}}
			/>
		</>
	);
};

export default EditUserPage;
