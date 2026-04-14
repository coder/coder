import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { deploymentConfig } from "#/api/queries/deployment";
import { roles } from "#/api/queries/roles";
import { updateProfile, updateRoles, user } from "#/api/queries/users";
import type { UpdateUserProfileRequest } from "#/api/typesGenerated";
import { Loader } from "#/components/Loader/Loader";
import { Margins } from "#/components/Margins/Margins";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { pageTitle } from "#/utils/page";
import { isUUID } from "#/utils/uuid";
import { EditUserForm } from "./EditUserForm";

const EditUserPage: FC = () => {
	const { user: usernameOrId } = useParams() as { user: string };
	const navigate = useNavigate();
	const queryClient = useQueryClient();

	const { permissions } = useAuthenticated();
	const { updateUsers: canEditRoles, viewDeploymentConfig } = permissions;

	const userQuery = useQuery(user(usernameOrId));
	const rolesQuery = useQuery(roles());
	const { data: deploymentValues } = useQuery({
		...deploymentConfig(),
		enabled: viewDeploymentConfig,
	});
	const updateProfileMutation = useMutation(
		updateProfile(userQuery.data?.id ?? ""),
	);
	const updateRolesMutation = useMutation(updateRoles(queryClient));

	if (!userQuery.data) {
		return <Loader />;
	}

	const userData = userQuery.data;

	// Indicates if OIDC roles are synced from the OIDC IdP.
	// Defaults to false when deployment config has not loaded yet.
	const oidcRoleSyncEnabled =
		viewDeploymentConfig &&
		(deploymentValues?.config.oidc?.user_role_field ?? "") !== "";

	const handleSubmit = async (values: UpdateUserProfileRequest) => {
		const mutation = updateProfileMutation.mutateAsync(values, {
			onSuccess: (updatedUser) => {
				// Invalidate the user cache so other parts of the UI reflect the change.
				void queryClient.invalidateQueries({
					queryKey: ["user", usernameOrId],
				});
				void queryClient.invalidateQueries({ queryKey: ["users"] });

				// If the URL currently uses the username (not a UUID) and the username
				// has changed, rewrite the URL so the page doesn't 404 on refresh.
				if (!isUUID(usernameOrId) && updatedUser.username !== usernameOrId) {
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
			error: (e) => ({
				message: getErrorMessage(
					e,
					`Failed to update user "${values.username}".`,
				),
				description: getErrorDetail(e),
			}),
		});
	};

	const handleUpdateRoles = async (newRoles: string[]) => {
		try {
			await updateRolesMutation.mutateAsync({
				userId: userData.id,
				roles: newRoles,
			});
			// Refetch the user so role pills update immediately.
			void queryClient.invalidateQueries({
				queryKey: ["user", usernameOrId],
			});
			toast.success("User roles updated successfully.");
		} catch (e) {
			toast.error(getErrorMessage(e, "Error updating user roles."), {
				description: getErrorDetail(e),
			});
		}
	};

	return (
		<Margins>
			<title>{pageTitle("Edit User", `${userData.username}`)}</title>

			<EditUserForm
				error={updateProfileMutation.error}
				isLoading={updateProfileMutation.isPending}
				initialValues={{
					username: userData.username,
					name: userData.name ?? "",
				}}
				onSubmit={handleSubmit}
				onCancel={() => {
					navigate("..", { relative: "path" });
				}}
				user={userData}
				availableRoles={rolesQuery.data}
				canEditRoles={canEditRoles}
				oidcRoleSyncEnabled={oidcRoleSyncEnabled}
				isUpdatingRoles={updateRolesMutation.isPending}
				onUpdateRoles={handleUpdateRoles}
			/>
		</Margins>
	);
};

export default EditUserPage;
