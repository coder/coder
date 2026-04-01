import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { updateProfile, user } from "#/api/queries/users";
import type { UpdateUserProfileRequest } from "#/api/typesGenerated";
import { Loader } from "#/components/Loader/Loader";
import { Margins } from "#/components/Margins/Margins";
import { pageTitle } from "#/utils/page";
import { isUUID } from "#/utils/uuid";
import { EditUserForm } from "./EditUserForm";

const EditUserPage: FC = () => {
	const { user: usernameOrId } = useParams() as { user: string };
	const navigate = useNavigate();
	const queryClient = useQueryClient();

	const userQuery = useQuery(user(usernameOrId));
	const updateProfileMutation = useMutation(
		updateProfile(userQuery.data?.id ?? ""),
	);

	if (!userQuery.data) {
		return <Loader />;
	}

	const userData = userQuery.data;

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
			/>
		</Margins>
	);
};

export default EditUserPage;
