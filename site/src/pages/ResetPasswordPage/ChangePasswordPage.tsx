import type { Interpolation, Theme } from "@emotion/react";
import LoadingButton from "@mui/lab/LoadingButton";
import Button from "@mui/material/Button";
import TextField from "@mui/material/TextField";
import { isApiError, isApiValidationError } from "api/errors";
import { changePasswordWithOTP } from "api/queries/users";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { CustomLogo } from "components/CustomLogo/CustomLogo";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Stack } from "components/Stack/Stack";
import { useFormik } from "formik";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation } from "react-query";
import {
	Link as RouterLink,
	useNavigate,
	useSearchParams,
} from "react-router-dom";
import { getApplicationName } from "utils/appearance";
import { getFormHelpers } from "utils/formUtils";
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
			<Helmet>
				<title>Reset Password - {applicationName}</title>
			</Helmet>

			<div css={styles.root}>
				<main css={styles.container}>
					<CustomLogo css={styles.logo} />
					<h1
						css={{
							margin: 0,
							marginBottom: 24,
							fontSize: 20,
							fontWeight: 600,
							lineHeight: "28px",
						}}
					>
						Choose a new password
					</h1>
					{changePasswordMutation.error &&
					!isApiValidationError(changePasswordMutation.error) ? (
						<ErrorAlert
							error={changePasswordMutation.error}
							css={{ marginBottom: 24 }}
						/>
					) : null}
					<form css={{ width: "100%" }} onSubmit={form.handleSubmit}>
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
									<LoadingButton
										loading={form.isSubmitting}
										type="submit"
										size="large"
										fullWidth
										variant="contained"
									>
										Reset password
									</LoadingButton>
									<Button
										component={RouterLink}
										size="large"
										fullWidth
										variant="text"
										to="/login"
									>
										Back to login
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
