import { isApiValidationError } from "api/errors";
import { changePasswordWithOTP } from "api/queries/users";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { CustomLogo } from "components/CustomLogo/CustomLogo";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { Spinner } from "components/Spinner/Spinner";
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
	const passwordField = getFieldHelpers("password");
	const confirmPasswordField = getFieldHelpers("confirmPassword");

	return (
		<>
			<title>{pageTitle("Reset Password", applicationName)}</title>

			<div className="p-6 flex items-center justify-center flex-col min-h-full text-center">
				<main className="w-full max-w-xs flex flex-col items-center">
					<div className="mb-10">
						<CustomLogo />
					</div>
					<h1 className="m-0 mb-6 text-xl font-semibold leading-7">
						Choose a new password
					</h1>
					{changePasswordMutation.error &&
					!isApiValidationError(changePasswordMutation.error) ? (
						<ErrorAlert error={changePasswordMutation.error} className="mb-6" />
					) : null}
					<form
						className="flex flex-col gap-5 w-full"
						onSubmit={form.handleSubmit}
					>
						<fieldset
							disabled={form.isSubmitting}
							className="flex flex-col gap-5"
						>
							<div className="flex flex-col items-start gap-2">
								<Label htmlFor={passwordField.id}>
									Password{" "}
									<span className="text-xs text-content-destructive font-bold">
										*
									</span>
								</Label>
								<Input
									id={passwordField.id}
									name={passwordField.name}
									value={passwordField.value}
									onChange={passwordField.onChange}
									onBlur={passwordField.onBlur}
									autoFocus
									required
									type="password"
									aria-invalid={passwordField.error}
								/>
								{passwordField.error && (
									<span className="text-xs text-content-destructive">
										{passwordField.helperText}
									</span>
								)}
							</div>

							<div className="flex flex-col items-start gap-2">
								<Label htmlFor={confirmPasswordField.id}>
									Confirm password{" "}
									<span className="text-xs text-content-destructive font-bold">
										*
									</span>
								</Label>
								<Input
									id={confirmPasswordField.id}
									name={confirmPasswordField.name}
									value={confirmPasswordField.value}
									onChange={confirmPasswordField.onChange}
									onBlur={confirmPasswordField.onBlur}
									required
									type="password"
									aria-invalid={confirmPasswordField.error}
								/>
								{confirmPasswordField.error && (
									<span className="text-xs text-content-destructive text-left">
										{confirmPasswordField.helperText}
									</span>
								)}
							</div>

							<div className="flex flex-col gap-2">
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
							</div>
						</fieldset>
					</form>
				</main>
			</div>
		</>
	);
};

export default ChangePasswordPage;
