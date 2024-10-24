import { authMethods, createUser, validatePassword } from "api/queries/users";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Margins } from "components/Margins/Margins";
import { useDebouncedFunction } from "hooks/debounce";
import { type FC, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router-dom";
import { pageTitle } from "utils/page";
import { CreateUserForm } from "./CreateUserForm";

export const Language = {
	unknownError: "Oops, an unknown error occurred.",
};

export const CreateUserPage: FC = () => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const createUserMutation = useMutation(createUser(queryClient));
	const authMethodsQuery = useQuery(authMethods());
	const validatePasswordMutation = useMutation(validatePassword());

	const [passwordIsValid, setPasswordIsValid] = useState(false);

	const validateUserPassword = async (password: string) => {
		validatePasswordMutation.mutate(password, {
			onSuccess: (data) => {
				setPasswordIsValid(data);
			},
		});
	};

	const { debounced: debouncedValidateUserPassword } = useDebouncedFunction(
		validateUserPassword,
		500,
	);

	return (
		<Margins>
			<Helmet>
				<title>{pageTitle("Create User")}</title>
			</Helmet>

			<CreateUserForm
				error={createUserMutation.error}
				authMethods={authMethodsQuery.data}
				onSubmit={async (user) => {
					await createUserMutation.mutateAsync(user);
					displaySuccess("Successfully created user.");
					navigate("..", { relative: "path" });
				}}
				onCancel={() => {
					navigate("..", { relative: "path" });
				}}
				onPasswordChange={debouncedValidateUserPassword}
				passwordIsValid={passwordIsValid}
				isLoading={createUserMutation.isLoading}
			/>
		</Margins>
	);
};

export default CreateUserPage;
