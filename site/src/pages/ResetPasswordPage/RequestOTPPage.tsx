import { requestOneTimePassword } from "api/queries/users";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { CustomLogo } from "components/CustomLogo/CustomLogo";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { Spinner } from "components/Spinner/Spinner";
import { useFormik } from "formik";
import type { FC } from "react";
import { useMutation } from "react-query";
import { Link as RouterLink, useSearchParams } from "react-router";
import { getApplicationName } from "utils/appearance";
import { getFormHelpers, onChangeTrimmed } from "utils/formUtils";
import { pageTitle } from "utils/page";
import * as Yup from "yup";

const RequestOTPPage: FC = () => {
	const applicationName = getApplicationName();
	const requestOTPMutation = useMutation(requestOneTimePassword());
	const [searchParams] = useSearchParams();
	const initialEmail = searchParams.get("email") ?? "";

	return (
		<>
			<title>{pageTitle("Reset Password", applicationName)}</title>

			<main className="p-6 flex items-center justify-center flex-col min-h-full text-center">
				<div>
					<CustomLogo />
				</div>
				{requestOTPMutation.isSuccess ? (
					<RequestOTPSuccess
						email={requestOTPMutation.variables?.email ?? ""}
					/>
				) : (
					<RequestOTP
						error={requestOTPMutation.error}
						isRequesting={requestOTPMutation.isPending}
						initialEmail={initialEmail}
						onRequest={(email) => {
							requestOTPMutation.mutate({ email });
						}}
					/>
				)}
			</main>
		</>
	);
};

type RequestOTPProps = {
	error: unknown;
	onRequest: (email: string) => void;
	isRequesting: boolean;
	initialEmail: string;
};

const validationSchema = Yup.object({
	email: Yup.string()
		.trim()
		.email("Please enter a valid email address.")
		.required("Please enter an email address."),
});

const RequestOTP: FC<RequestOTPProps> = ({
	error,
	onRequest,
	isRequesting,
	initialEmail,
}) => {
	const form = useFormik({
		initialValues: { email: initialEmail },
		validationSchema,
		validateOnBlur: false,
		onSubmit: (values) => {
			onRequest(values.email);
		},
	});
	const getFieldHelpers = getFormHelpers(form);
	const emailField = getFieldHelpers("email");

	return (
		<div className="w-full max-w-xs flex flex-col items-center">
			<div>
				<h1 className="m-0 mb-6 text-xl font-semibold leading-7">
					Enter your email to reset the password
				</h1>
				{error ? <ErrorAlert error={error} className="mb-6" /> : null}
				<form
					className="flex flex-col gap-5 w-full"
					onSubmit={form.handleSubmit}
				>
					<fieldset disabled={isRequesting} className="flex flex-col gap-5">
						<div className="flex flex-col items-start gap-2">
							<Label htmlFor={emailField.id}>
								Email{" "}
								<span className="text-xs text-content-destructive font-bold">
									*
								</span>
							</Label>
							<Input
								id={emailField.id}
								name={emailField.name}
								value={emailField.value}
								onChange={onChangeTrimmed(form)}
								onBlur={emailField.onBlur}
								type="email"
								autoFocus
								aria-invalid={emailField.error}
							/>
							{emailField.error && (
								<span className="text-xs text-content-destructive">
									{emailField.helperText}
								</span>
							)}
						</div>

						<div className="flex flex-col gap-2">
							<Button
								disabled={isRequesting}
								type="submit"
								size="lg"
								className="w-full"
							>
								<Spinner loading={isRequesting} />
								Reset password
							</Button>
							<Button asChild size="lg" variant="outline" className="w-full">
								<RouterLink to="/login">Cancel</RouterLink>
							</Button>
						</div>
					</fieldset>
				</form>
			</div>
		</div>
	);
};

const RequestOTPSuccess: FC<{ email: string }> = ({ email }) => {
	return (
		<div className="w-full max-w-[380px] flex flex-col items-center font-medium text-sm leading-6">
			<div>
				<p className="m-0 mb-14">
					If the account{" "}
					<span className="font-semibold text-content-secondary">{email}</span>{" "}
					exists, you will get an email with instructions on resetting your
					password.
				</p>

				<p className="m-0 text-xs leading-4 text-content-secondary mb-12">
					Contact your deployment administrator if you encounter issues.
				</p>

				<Button asChild variant="default">
					<RouterLink to="/login">Back to login</RouterLink>
				</Button>
			</div>
		</div>
	);
};

export default RequestOTPPage;
