import Link from "@mui/material/Link";
import TextField from "@mui/material/TextField";
import { Button } from "components/Button/Button";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
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
		<form onSubmit={form.handleSubmit}>
			<Stack spacing={2.5}>
				<TextField
					{...getFieldHelpers("email")}
					onChange={onChangeTrimmed(form)}
					autoFocus={autoFocus}
					autoComplete="email"
					fullWidth
					label={Language.emailLabel}
					type="email"
				/>
				<TextField
					{...getFieldHelpers("password")}
					autoComplete="current-password"
					fullWidth
					id="password"
					label={Language.passwordLabel}
					type="password"
				/>
				<Button
					size="lg"
					disabled={isSigningIn}
					className="w-full"
					type="submit"
				>
					<Spinner loading={isSigningIn} />
					{Language.passwordSignIn}
				</Button>
				<Link
					component={RouterLink}
					to="/reset-password"
					css={{
						fontSize: 12,
						fontWeight: 500,
						lineHeight: "16px",
					}}
				>
					Forgot password?
				</Link>
			</Stack>
		</form>
	);
};
