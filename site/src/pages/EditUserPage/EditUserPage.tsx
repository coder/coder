import { getErrorDetail, getErrorMessage } from "api/errors";
import { user } from "api/queries/users";
import { Margins } from "components/Margins/Margins";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { pageTitle } from "utils/page";
import { EditUserForm } from "./EditUserForm";

const EditUserPage: FC = () => {
	const { userId } = useParams() as { userId: string };
	const navigate = useNavigate();
	const userQuery = useQuery(user(userId));

	return (
		<Margins>
			<title>{pageTitle("Edit User")}</title>

			<h1>{userQuery.data?.name}</h1>

			{/*<EditUserForm
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
			/>*/}
		</Margins>
	);
};

export default EditUserPage;
