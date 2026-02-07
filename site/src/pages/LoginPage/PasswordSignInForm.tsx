import { Button } from "components/Button/Button";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { Link } from "components/Link/Link";
import { Spinner } from "components/Spinner/Spinner";
import { useFormik } from "formik";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { getFormHelpers, onChangeTrimmed } from "utils/formUtils";
import * as Yup from "yup";
import { Language } from "./Language";

type PasswordSignInFormProps = {
	onSubmit: (credentials: { email: string; password: string }) => void;
	isSigningIn: boolean;
	autoFocus: boolean;
};

export const PasswordSignInForm: FC<PasswordSignInFormProps> = ({
	onSubmit,
	isSigningIn,
	autoFocus,
}) => {
	const validationSchema = Yup.object({
		email: Yup.string()
			.trim()
			.email(Language.emailInvalid)
			.required(Language.emailRequired),
		password: Yup.string(),
	});

	const form = useFormik({
		initialValues: {
			email: "",
			password: "",
		},
		validationSchema,
		onSubmit,
		validateOnBlur: false,
	});
	const getFieldHelpers = getFormHelpers(form);
	const emailField = getFieldHelpers("email");
	const passwordField = getFieldHelpers("password");

	return (
		<form onSubmit={form.handleSubmit} className="flex flex-col gap-5">
			<div className="flex flex-col items-start gap-2">
				<Label htmlFor={emailField.id}>
					{Language.emailLabel}{" "}
					<span className="text-xs text-content-destructive font-bold">*</span>
				</Label>
				<Input
					id={emailField.id}
					name={emailField.name}
					value={emailField.value}
					onChange={onChangeTrimmed(form)}
					onBlur={emailField.onBlur}
					autoFocus={autoFocus}
					autoComplete="email"
					type="email"
					aria-invalid={Boolean(emailField.error)}
				/>
				{emailField.error && (
					<span className="text-xs text-content-destructive">
						{emailField.helperText}
					</span>
				)}
			</div>

			<div className="flex flex-col items-start gap-2">
				<Label htmlFor={passwordField.id}>
					{Language.passwordLabel}{" "}
					<span className="text-xs text-content-destructive font-bold">*</span>
				</Label>
				<Input
					id={passwordField.id}
					name={passwordField.name}
					value={passwordField.value}
					onChange={passwordField.onChange}
					onBlur={passwordField.onBlur}
					autoComplete="current-password"
					type="password"
					aria-invalid={passwordField.error}
				/>
				{passwordField.error && (
					<span className="text-xs text-content-destructive">
						{passwordField.helperText}
					</span>
				)}
			</div>

			<Button size="lg" disabled={isSigningIn} className="w-full" type="submit">
				<Spinner loading={isSigningIn} />
				{Language.passwordSignIn}
			</Button>

			<Link
				asChild
				size="sm"
				showExternalIcon={false}
				className="flex items-center justify-center"
			>
				<RouterLink
					to={
						form.values.email
							? `/reset-password?email=${encodeURIComponent(form.values.email)}`
							: "/reset-password"
					}
					className="mx-auto"
				>
					Forgot password?
				</RouterLink>
			</Link>
		</form>
	);
};
