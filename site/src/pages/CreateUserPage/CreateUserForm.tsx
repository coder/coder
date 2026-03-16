import * as SelectPrimitive from "@radix-ui/react-select";
import { hasApiFieldErrors, isApiError } from "api/errors";
import type * as TypesGen from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { FormField, FormFooter } from "components/Form/Form";
import { FullPageForm } from "components/FullPageForm/FullPageForm";
import { Label } from "components/Label/Label";
import { Link } from "components/Link/Link";
import { OrganizationAutocomplete } from "components/OrganizationAutocomplete/OrganizationAutocomplete";
import {
	Select,
	SelectContent,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { Spinner } from "components/Spinner/Spinner";
import { useFormik } from "formik";
import { Check } from "lucide-react";
import type { FC } from "react";
import { cn } from "utils/cn";
import {
	displayNameValidator,
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "utils/formUtils";
import * as Yup from "yup";

const loginTypeOptions = {
	password: {
		label: "Password",
		description: "Use an email address and password to log in.",
	},
	oidc: {
		label: "OpenID Connect",
		description: "Use an OpenID Connect provider for authentication.",
	},
	github: {
		label: "GitHub",
		description: "Use GitHub OAuth for authentication.",
	},
	none: {
		label: "Service account",
		description: (
			<>
				Cannot log in interactively. Intended for automated pipelines, bots, and
				other non-human access.{" "}
				<Link
					target="_blank"
					rel="noopener"
					href="https://coder.com/docs/admin/users/headless-auth"
				>
					See the documentation.
				</Link>
			</>
		),
	},
} as const;

type LoginTypeKey = keyof typeof loginTypeOptions;

const validationSchema = Yup.object({
	username: nameValidator("Username"),
	name: displayNameValidator("Full name"),
	email: Yup.string()
		.trim()
		.when("service_account", {
			is: false,
			then: (schema) =>
				schema
					.email("Please enter a valid email address.")
					.required("Please enter an email address."),
			otherwise: (schema) => schema.optional(),
		}),
	login_type: Yup.string()
		.oneOf(Object.keys(loginTypeOptions))
		.required("Please select a login type."),
	password: Yup.string().when("login_type", {
		is: "password",
		then: (schema) => schema.required("Please enter a password."),
		otherwise: (schema) => schema,
	}),
});

export type CreateUserFormData = {
	readonly username: string;
	readonly name: string;
	readonly email: string;
	readonly organization: string;
	readonly login_type: TypesGen.LoginType;
	readonly password: string;
	readonly service_account: boolean;
};

interface CreateUserFormProps {
	error?: unknown;
	isLoading: boolean;
	onSubmit: (user: CreateUserFormData) => void;
	onCancel: () => void;
	authMethods?: TypesGen.AuthMethods;
	showOrganizations: boolean;
}

export const CreateUserForm: FC<CreateUserFormProps> = ({
	error,
	isLoading,
	onSubmit,
	onCancel,
	showOrganizations,
	authMethods,
}) => {
	const form = useFormik<CreateUserFormData>({
		initialValues: {
			email: "",
			password: "",
			username: "",
			name: "",
			organization: showOrganizations
				? ""
				: "00000000-0000-0000-0000-000000000000",
			login_type: "",
			service_account: false,
		},
		validationSchema,
		onSubmit,
	});

	const getFieldHelpers = getFormHelpers(form, error);

	const availableLoginTypeKeys = (
		Object.keys(loginTypeOptions) as LoginTypeKey[]
	).filter((key) => {
		if (key === "none") return true;
		if (key === "password") return authMethods?.password.enabled;
		if (key === "oidc") return authMethods?.oidc.enabled;
		if (key === "github") return authMethods?.github.enabled;
		return false;
	});

	const isServiceAccount = form.values.login_type === "none";
	const isPasswordLogin = form.values.login_type === "password";
	const loginTypeField = getFieldHelpers("login_type", {
		helperText: "Authentication method for this user.",
	});

	return (
		<FullPageForm title="Create user">
			{isApiError(error) && !hasApiFieldErrors(error) && (
				<ErrorAlert error={error} css={{ marginBottom: 32 }} />
			)}
			<form onSubmit={form.handleSubmit} autoComplete="off">
				<div className="flex flex-col gap-6">
					<FormField
						field={getFieldHelpers("username")}
						label="Username"
						id="username"
						name="username"
						value={form.values.username}
						onChange={onChangeTrimmed(form)}
						onBlur={form.handleBlur}
						autoComplete="username"
						autoFocus
					/>

					<FormField
						field={getFieldHelpers("name")}
						label={
							<>
								Full name{" "}
								<span className="font-normal text-content-secondary">
									(optional)
								</span>
							</>
						}
						id="name"
						name="name"
						value={form.values.name}
						onChange={form.handleChange}
						onBlur={form.handleBlur}
						autoComplete="name"
					/>

					{/* Login type — "none" is presented as "Service account" */}
					<div className="flex flex-col gap-2">
						<Label htmlFor="login_type">Login type</Label>
						<Select
							value={form.values.login_type}
							onValueChange={async (value) => {
								const isNone = value === "none";
								await Promise.all([
									form.setFieldValue("login_type", value),
									form.setFieldValue("service_account", isNone),
									value !== "password"
										? form.setFieldValue("password", "")
										: Promise.resolve(),
								]);
							}}
						>
							<SelectTrigger
								id="login_type"
								data-testid="login-type-input"
								aria-invalid={loginTypeField.error}
								aria-describedby={
									loginTypeField.error
										? "login_type-error"
										: "login_type-helper"
								}
								className={cn(
									loginTypeField.error && "border-border-destructive",
								)}
							>
								<SelectValue placeholder="Select a login type…" />
							</SelectTrigger>
							<SelectContent>
								{availableLoginTypeKeys.map((key) => {
									const opt = loginTypeOptions[key];
									return (
										<SelectPrimitive.Item
											key={key}
											value={key}
											className="relative flex w-full cursor-default select-none items-start rounded-sm py-1.5 pl-2 pr-8 text-sm text-content-secondary outline-none focus:bg-surface-secondary focus:text-content-primary data-[disabled]:pointer-events-none data-[disabled]:opacity-50"
										>
											<span className="absolute right-2 top-2 flex items-center justify-center">
												<SelectPrimitive.ItemIndicator>
													<Check className="size-icon-sm" />
												</SelectPrimitive.ItemIndicator>
											</span>
											<div className="flex flex-col py-0.5">
												<SelectPrimitive.ItemText>
													{opt.label}
												</SelectPrimitive.ItemText>
												<span className="text-xs text-content-secondary whitespace-normal">
													{opt.description}
												</span>
											</div>
										</SelectPrimitive.Item>
									);
								})}
							</SelectContent>
						</Select>
						{loginTypeField.helperText && (
							<span
								id="login_type-helper"
								className="text-xs text-content-secondary"
							>
								{loginTypeField.helperText}
							</span>
						)}
					</div>

					{!isServiceAccount && (
						<FormField
							field={getFieldHelpers("email")}
							label={
								<>
									Email{" "}
									<span className="text-xs font-bold text-content-destructive">
										*
									</span>
								</>
							}
							id="email"
							name="email"
							value={form.values.email}
							onChange={onChangeTrimmed(form)}
							onBlur={form.handleBlur}
							autoComplete="email"
							type="email"
						/>
					)}

					{showOrganizations && (
						<div className="flex flex-col gap-2">
							<Label htmlFor="organization">Organization</Label>
							<OrganizationAutocomplete
								{...getFieldHelpers("organization")}
								id="organization"
								required
								onChange={(newValue) => {
									void form.setFieldValue("organization", newValue?.id ?? "");
								}}
								check={{
									object: { resource_type: "organization_member" },
									action: "create",
								}}
							/>
						</div>
					)}

					{isPasswordLogin && (
						<FormField
							field={getFieldHelpers("password")}
							label="Password"
							id="password"
							name="password"
							value={form.values.password}
							onChange={form.handleChange}
							onBlur={form.handleBlur}
							autoComplete="new-password"
							type="password"
							data-testid="password-input"
						/>
					)}
				</div>

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
