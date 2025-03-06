import Link from "@mui/material/Link";
import MenuItem from "@mui/material/MenuItem";
import TextField from "@mui/material/TextField";
import { hasApiFieldErrors, isApiError } from "api/errors";
import type * as TypesGen from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { FormFooter } from "components/Form/Form";
import { FullPageForm } from "components/FullPageForm/FullPageForm";
import { OrganizationAutocomplete } from "components/OrganizationAutocomplete/OrganizationAutocomplete";
import { PasswordField } from "components/PasswordField/PasswordField";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import { type FormikContextType, useFormik } from "formik";
import type { FC } from "react";
import {
	displayNameValidator,
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "utils/formUtils";
import * as Yup from "yup";
import { Language } from "./Language";

export const authMethodLanguage = {
	password: {
		displayName: "Password",
		description: "Use an email address and password to login",
	},
	oidc: {
		displayName: "OpenID Connect",
		description: "Use an OpenID Connect provider for authentication",
	},
	github: {
		displayName: "Github",
		description: "Use Github OAuth for authentication",
	},
	none: {
		displayName: "None",
		description: (
			<>
				Disable authentication for this user (See the{" "}
				<Link
					target="_blank"
					rel="noopener"
					href="https://coder.com/docs/admin/users/headless-auth"
				>
					documentation
				</Link>{" "}
				for more details)
			</>
		),
	},
};

const validationSchema = Yup.object({
	email: Yup.string()
		.trim()
		.email(Language.emailInvalid)
		.required(Language.emailRequired),
	password: Yup.string().when("login_type", {
		is: "password",
		then: (schema) => schema.required(Language.passwordRequired),
		otherwise: (schema) => schema,
	}),
	username: nameValidator(Language.usernameLabel),
	name: displayNameValidator(Language.nameLabel),
	login_type: Yup.string().oneOf(Object.keys(authMethodLanguage)),
});

type CreateUserFormData = {
	readonly username: string;
	readonly name: string;
	readonly email: string;
	readonly organization: string;
	readonly login_type: TypesGen.LoginType;
	readonly password: string;
};

export interface CreateUserFormProps {
	error?: unknown;
	isLoading: boolean;
	onSubmit: (user: CreateUserFormData) => void;
	onCancel: () => void;
	authMethods?: TypesGen.AuthMethods;
	organizations?: readonly TypesGen.Organization[];
}

export const CreateUserForm: FC<
	React.PropsWithChildren<CreateUserFormProps>
> = ({ error, isLoading, onSubmit, onCancel, organizations, authMethods }) => {
	const form = useFormik<CreateUserFormData>({
		initialValues: {
			email: "",
			password: "",
			username: "",
			name: "",
			// If we aren't given a list of organizations to choose from, use the
			// fallback ID to add the user to the default organization.
			organization: organizations ? "" : "00000000-0000-0000-0000-000000000000",
			login_type: "",
		},
		validationSchema,
		onSubmit,
	});
	const getFieldHelpers = getFormHelpers(form, error);

	const methods = [
		authMethods?.password.enabled && "password",
		authMethods?.oidc.enabled && "oidc",
		authMethods?.github.enabled && "github",
		"none",
	].filter(Boolean) as Array<keyof typeof authMethodLanguage>;

	return (
		<FullPageForm title="Create user">
			{isApiError(error) && !hasApiFieldErrors(error) && (
				<ErrorAlert error={error} css={{ marginBottom: 32 }} />
			)}
			<form onSubmit={form.handleSubmit} autoComplete="off">
				<Stack spacing={2.5}>
					<TextField
						{...getFieldHelpers("username")}
						onChange={onChangeTrimmed(form)}
						autoComplete="username"
						autoFocus
						fullWidth
						label={Language.usernameLabel}
					/>
					<TextField
						{...getFieldHelpers("name")}
						autoComplete="name"
						fullWidth
						label={Language.nameLabel}
					/>
					<TextField
						{...getFieldHelpers("email")}
						onChange={onChangeTrimmed(form)}
						autoComplete="email"
						fullWidth
						label={Language.emailLabel}
					/>
					{organizations && (
						<OrganizationAutocomplete
							{...getFieldHelpers("organization")}
							required
							label="Organization"
							value={
								organizations.find(
									(it) => it.id === form.values.organization,
								) ?? null
							}
							onChange={(newValue) => {
								void form.setFieldValue("organization", newValue?.id ?? "");
							}}
							check={{
								object: { resource_type: "organization_member" },
								action: "create",
							}}
						/>
					)}
					<TextField
						{...getFieldHelpers("login_type", {
							helperText: "Authentication method for this user",
						})}
						select
						id="login_type"
						data-testid="login-type-input"
						value={form.values.login_type}
						label="Login Type"
						onChange={async (e) => {
							if (e.target.value !== "password") {
								await form.setFieldValue("password", "");
							}
							await form.setFieldValue("login_type", e.target.value);
						}}
						SelectProps={{
							renderValue: (selected: unknown) =>
								authMethodLanguage[selected as keyof typeof authMethodLanguage]
									?.displayName ?? "",
						}}
					>
						{methods.map((value) => {
							const language = authMethodLanguage[value];
							return (
								<MenuItem key={value} id={`item-${value}`} value={value}>
									<Stack
										spacing={0}
										css={{
											maxWidth: 400,
										}}
									>
										{language.displayName}
										<span
											css={(theme) => ({
												fontSize: 14,
												color: theme.palette.text.secondary,
												wordWrap: "normal",
												whiteSpace: "break-spaces",
											})}
										>
											{language.description}
										</span>
									</Stack>
								</MenuItem>
							);
						})}
					</TextField>
					<PasswordField
						{...getFieldHelpers("password", {
							helperText:
								form.values.login_type !== "password" &&
								"No password required for this login type",
						})}
						autoComplete="current-password"
						fullWidth
						id="password"
						data-testid="password-input"
						disabled={form.values.login_type !== "password"}
						label={Language.passwordLabel}
					/>
				</Stack>

				<FormFooter className="mt-8">
					<Button onClick={onCancel} variant="outline">
						Cancel
					</Button>
					<Button type="submit" disabled={isLoading}>
						<Spinner loading={isLoading} />
						Save
					</Button>
				</FormFooter>
			</form>
		</FullPageForm>
	);
};
