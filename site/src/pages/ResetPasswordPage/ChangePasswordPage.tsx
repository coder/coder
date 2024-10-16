import type { Interpolation, Theme } from "@emotion/react";
import LoadingButton from "@mui/lab/LoadingButton";
import { Button, TextField } from "@mui/material";
import { CustomLogo } from "components/CustomLogo/CustomLogo";
import { Stack } from "components/Stack/Stack";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { getApplicationName } from "utils/appearance";
import {
	Link as RouterLink,
	useNavigate,
	useSearchParams,
} from "react-router-dom";
import { useMutation } from "react-query";
import { changePasswordWithOTP } from "api/queries/users";
import { getErrorMessage } from "api/errors";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { useFormik } from "formik";
import * as yup from "yup";
import { getFormHelpers } from "utils/formUtils";

const validationSchema = yup.object({
	password: yup.string().required("Password is required"),
	confirmPassword: yup
		.string()
		.required("Confirm password is required")
		.test("passwords-match", "Passwords must match", function (value) {
			return this.parent.password === value;
		}),
});

const ChangePasswordPage: FC = () => {
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

			try {
				await changePasswordMutation.mutateAsync({
					email,
					one_time_passcode: otp,
					password: values.password,
				});
				displaySuccess("Password reset successfully");
				navigate("/login");
			} catch (error) {
				displayError(getErrorMessage(error, "Error resetting password"));
			}
		},
	});
	const getFieldHelpers = getFormHelpers(form);

	return (
		<>
			<Helmet>
				<title>Reset Password - {applicationName}</title>
			</Helmet>

			<div css={styles.root}>
				<main css={styles.container}>
					<CustomLogo />
					<h1
						css={{
							fontSize: 20,
							fontWeight: 600,
							lineHeight: "28px",
						}}
					>
						Choose a new password
					</h1>
					<form css={{ width: "100%" }} onSubmit={form.handleSubmit}>
						<fieldset disabled={form.isSubmitting}>
							<Stack spacing={2.5}>
								<TextField
									label="New password"
									autoFocus
									fullWidth
									required
									type="password"
									{...getFieldHelpers("password")}
								/>

								<TextField
									label="Confirm new password"
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
										Cancel
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
	root: {
		padding: 24,
		display: "flex",
		alignItems: "center",
		justifyContent: "center",
		minHeight: "100%",
		textAlign: "center",
	},
	container: {
		width: "100%",
		maxWidth: 320,
		display: "flex",
		flexDirection: "column",
		alignItems: "center",
		gap: 16,
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
