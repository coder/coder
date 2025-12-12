import type { Interpolation, Theme } from "@emotion/react";
import TextField from "@mui/material/TextField";
import { isApiValidationError } from "api/errors";
import { changePasswordWithOTP } from "api/queries/users";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { CustomLogo } from "components/CustomLogo/CustomLogo";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import { useFormik } from "formik";
import type { FC } from "react";
import { useMutation } from "react-query";
import { Link as RouterLink, useNavigate, useSearchParams } from "react-router";
import { getApplicationName } from "utils/appearance";
import { getFormHelpers } from "utils/formUtils";
import { pageTitle } from "utils/page";
import * as yup from "yup";

const validationSchema = yup.object({
	password: yup.string().required("Password is required"),
	confirmPassword: yup
		.string()
		.required("Confirm password is required")
		.test("passwords-match", "Passwords must match", function (value) {
			return this.parent.password === value;
		}),
});

type ChangePasswordChangeProps = {
	// This is used to prevent redirection when testing the page in Storybook and
	// capturing Chromatic snapshots.
	redirect?: boolean;
};

const ChangePasswordPage: FC<ChangePasswordChangeProps> = ({ redirect }) => {
	const navigate = useNavigate();
	const applicationName = getApplicationName();
	const changePasswordMutation = useMutation(changePasswordWithOTP());
	const [searchParams] = useSearchParams();

	const form = useFormik({
		initialValues: {
			password: "",
			confirmPassword: "",
		},
		validateOnBlur: false,
		validationSchema,
		onSubmit: async (values) => {
			const email = searchParams.get("email") ?? "";
			const otp = searchParams.get("otp") ?? "";

			await changePasswordMutation.mutateAsync({
				email,
				one_time_passcode: otp,
				password: values.password,
			});
			displaySuccess("Password reset successfully");
			if (redirect) {
				navigate("/login");
			}
		},
	});
	const getFieldHelpers = getFormHelpers(form, changePasswordMutation.error);

	return (
		<>
			<title>{pageTitle("Reset Password", applicationName)}</title>

			<div css={styles.root}>
				<main css={styles.container}>
					<CustomLogo css={styles.logo} />
					<h1 className="m-0 mb-6 text-xl font-semibold leading-7">
						Choose a new password
					</h1>
					{changePasswordMutation.error &&
					!isApiValidationError(changePasswordMutation.error) ? (
						<ErrorAlert error={changePasswordMutation.error} className="mb-6" />
					) : null}
					<form className="w-full" onSubmit={form.handleSubmit}>
						<fieldset disabled={form.isSubmitting}>
							<Stack spacing={2.5}>
								<TextField
									label="Password"
									autoFocus
									fullWidth
									required
									type="password"
									{...getFieldHelpers("password")}
								/>

								<TextField
									label="Confirm password"
									fullWidth
									required
									type="password"
									{...getFieldHelpers("confirmPassword")}
								/>

								<Stack spacing={1}>
									<Button
										disabled={form.isSubmitting}
										type="submit"
										size="lg"
										className="w-full"
									>
										<Spinner loading={form.isSubmitting} />
										Reset password
									</Button>
									<Button size="lg" className="w-full" variant="subtle" asChild>
										<RouterLink to="/login">Back to login</RouterLink>
									</Button>
								</Stack>
							</Stack>
						</fieldset>
					</form>
				</main>
			</div>
		</>
	);
};

const styles = {
	logo: {
		marginBottom: 40,
	},
	root: {
		padding: 24,
		display: "flex",
		alignItems: "center",
		justifyContent: "center",
		flexDirection: "column",
		minHeight: "100%",
		textAlign: "center",
	},
	container: {
		width: "100%",
		maxWidth: 320,
		display: "flex",
		flexDirection: "column",
		alignItems: "center",
	},
	icon: {
		fontSize: 64,
	},
	footer: (theme) => ({
		fontSize: 12,
		color: theme.palette.text.secondary,
		marginTop: 24,
	}),
} satisfies Record<string, Interpolation<Theme>>;

export default ChangePasswordPage;
