import { Button } from "components/Button/Button";
import { FormField } from "components/Form/Form";
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

	return (
		<form onSubmit={form.handleSubmit} className="flex flex-col gap-5">
			<FormField
				field={getFieldHelpers("email")}
				label={
					<>
						{Language.emailLabel}{" "}
						<span className="text-xs text-content-destructive font-bold">
							*
						</span>
					</>
				}
				id="email"
				name="email"
				value={form.values.email}
				onChange={onChangeTrimmed(form)}
				onBlur={form.handleBlur}
				autoFocus={autoFocus}
				autoComplete="email"
				type="email"
			/>

			<FormField
				field={getFieldHelpers("password")}
				label={
					<>
						{Language.passwordLabel}{" "}
						<span className="text-xs text-content-destructive font-bold">
							*
						</span>
					</>
				}
				id="password"
				name="password"
				value={form.values.password}
				onChange={form.handleChange}
				onBlur={form.handleBlur}
				autoComplete="current-password"
				type="password"
			/>

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
