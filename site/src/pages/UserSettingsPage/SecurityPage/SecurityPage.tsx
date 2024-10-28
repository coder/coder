import { API } from "api/api";
import {
	authMethods,
	updatePassword,
	validatePassword,
} from "api/queries/users";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useDebouncedFunction } from "hooks/debounce";
import { type ComponentProps, type FC, useState } from "react";
import { useMutation, useQuery } from "react-query";
import { Section } from "../Section";
import { SecurityForm } from "./SecurityForm";
import {
	SingleSignOnSection,
	useSingleSignOnSection,
} from "./SingleSignOnSection";

export const SecurityPage: FC = () => {
	const { user: me } = useAuthenticated();
	const updatePasswordMutation = useMutation(updatePassword());
	const validatePasswordMutation = useMutation(validatePassword());
	const authMethodsQuery = useQuery(authMethods());
	const { data: userLoginType } = useQuery({
		queryKey: ["loginType"],
		queryFn: API.getUserLoginType,
	});
	const singleSignOnSection = useSingleSignOnSection();

	const [passwordValidator, setPasswordValidator] = useState({
		valid: false,
		details: "",
	});

	const validateUserPassword = async (password: string) => {
		validatePasswordMutation.mutate(password, {
			onSuccess: (data) => {
				setPasswordValidator({ valid: data.valid, details: data.details });
			},
		});
	};

	const { debounced: debouncedValidateUserPassword } = useDebouncedFunction(
		validateUserPassword,
		500,
	);

	if (!authMethodsQuery.data || !userLoginType) {
		return <Loader />;
	}

	return (
		<SecurityPageView
			security={{
				form: {
					disabled: userLoginType.login_type !== "password",
					error: updatePasswordMutation.error,
					isLoading: updatePasswordMutation.isLoading,
					onPasswordChange: debouncedValidateUserPassword,
					passwordValidator: passwordValidator,
					onSubmit: async (data) => {
						await updatePasswordMutation.mutateAsync({
							userId: me.id,
							...data,
						});
						displaySuccess("Updated password.");
						// Refresh the browser session. We need to improve the AuthProvider
						// to include better API to handle these scenarios
						window.location.href = location.origin;
					},
				},
			}}
			oidc={{
				section: {
					authMethods: authMethodsQuery.data,
					userLoginType,
					...singleSignOnSection,
				},
			}}
		/>
	);
};

interface SecurityPageViewProps {
	security: {
		form: ComponentProps<typeof SecurityForm>;
	};
	oidc?: {
		section: ComponentProps<typeof SingleSignOnSection>;
	};
}

export const SecurityPageView: FC<SecurityPageViewProps> = ({
	security,
	oidc,
}) => {
	return (
		<Stack spacing={6}>
			<Section title="Security" description="Update your account password">
				<SecurityForm {...security.form} />
			</Section>
			{oidc && <SingleSignOnSection {...oidc.section} />}
		</Stack>
	);
};

export default SecurityPage;
