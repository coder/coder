import { authMethods, createUser } from "api/queries/users";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Margins } from "components/Margins/Margins";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router-dom";
import { pageTitle } from "utils/page";
import { CreateUserForm } from "./CreateUserForm";

const Language = {
	unknownError: "Oops, an unknown error occurred.",
};

export const CreateUserPage: FC = () => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const createUserMutation = useMutation(createUser(queryClient));
	const authMethodsQuery = useQuery(authMethods());
	const { showOrganizations } = useDashboard();

	return (
		<Margins>
			<Helmet>
				<title>{pageTitle("Create User")}</title>
			</Helmet>

			<CreateUserForm
				error={createUserMutation.error}
				isLoading={createUserMutation.isLoading}
				onSubmit={async (user) => {
					await createUserMutation.mutateAsync({
						username: user.username,
						name: user.name,
						email: user.email,
						organization_ids: [user.organization],
						login_type: user.login_type,
						password: user.password,
						user_status: null,
					});
					displaySuccess("Successfully created user.");
					navigate("..", { relative: "path" });
				}}
				onCancel={() => {
					navigate("..", { relative: "path" });
				}}
				authMethods={authMethodsQuery.data}
				showOrganizations={showOrganizations}
			/>
		</Margins>
	);
};

export default CreateUserPage;
