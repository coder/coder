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
	const [action, setAction] = useState<UserAdminAction>();

	const userQuery = useQuery(user(usernameOrId));
	const updateProfileMutation = useMutation(
		updateProfile(userQuery.data?.id ?? ""),
	);
	const { data: deploymentValues } = useQuery({
		...deploymentConfig(),
		enabled: permissions.viewDeploymentConfig,
	});

	const oidcRoleSyncEnabled = Boolean(
		permissions.viewDeploymentConfig &&
			deploymentValues?.config.oidc?.user_role_field,
	);

	const userData = userQuery.data;

	return (
		<>
			<title>
				{pageTitle(
					userData
						? `Edit ${userData.name?.trim() || userData.username}`
						: "Edit user",
				)}
			</title>

			{userQuery.isLoading ? (
				<Loader />
			) : !userData ? (
				<ErrorAlert error={userQuery.error} />
			) : (
				<EditUserForm
					error={updateProfileMutation.error}
					isLoading={updateProfileMutation.isPending}
					initialValues={{
						username: userData.username,
						name: userData.name ?? "",
						avatar_url: userData.avatar_url ?? "",
					}}
					canEditAvatar={
						userData.login_type === "password" || userData.login_type === "none"
					}
					headerActions={
						permissions.updateUsers && (
							<UserMoreActions
								user={userData}
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
								if (
									!isUUID(usernameOrId) &&
									updatedUser.username !== usernameOrId
								) {
									queryClient.removeQueries({
										queryKey: userKey(usernameOrId),
										exact: true,
									});
								}

								// The response is the saved user, so write it straight to the
								// cache: the heading updates immediately instead of waiting on
								// a refetch, and a rename lands on a warm cache entry.
								queryClient.setQueryData(userKey(updatedUser.id), updatedUser);
								queryClient.setQueryData(
									userKey(updatedUser.username),
									updatedUser,
								);
								void queryClient.invalidateQueries({
									queryKey: usersQueryKey,
								});

								// If the URL currently uses the username (not a UUID) and the
								// username has changed, rewrite the URL so the page doesn't
								// 404 on refresh.
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
							loading: `Saving user "${values.username}"…`,
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
