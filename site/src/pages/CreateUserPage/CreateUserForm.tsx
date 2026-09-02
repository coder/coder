import { useFormik } from "formik";
import { ArrowLeftIcon, CheckIcon } from "lucide-react";
import { Select as SelectPrimitive } from "radix-ui";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { Link } from "react-router";
import * as Yup from "yup";
import { hasApiFieldErrors, isApiError } from "#/api/errors";
import { permittedOrganizations } from "#/api/queries/organizations";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { FormFields, FormFooter } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { Label } from "#/components/Label/Label";
import { OrganizationAutocomplete } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import {
	Select,
	SelectContent,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import { RoleSelector } from "#/modules/roles/RoleSelector";
import { cn } from "#/utils/cn";
import {
	displayNameValidator,
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "#/utils/formUtils";

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
		description:
			"Cannot log in interactively. Intended for automated pipelines, bots, and other non-human access.",
	},
} as const;

const validationSchema = Yup.object({
	username: nameValidator("Username"),
	name: displayNameValidator("Name"),
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

type CreateUserFormData = {
	readonly username: string;
	readonly name: string;
	readonly email: string;
	readonly organization: string;
	readonly login_type: TypesGen.LoginType;
	readonly password: string;
	readonly service_account: boolean;
	readonly roles: Set<string>;
};

type CreateUserFormProps = {
	error?: unknown;
	isLoading: boolean;
	onSubmit: (user: CreateUserFormData) => void;
	onCancel: () => void;
	authMethods: TypesGen.AuthMethods;
	showOrganizations: boolean;
	serviceAccountsEnabled: boolean;
	availableRoles?: TypesGen.AssignableRoles[];
	rolesLoading?: boolean;
	rolesError?: unknown;
};

// Stable reference for empty org options to avoid re-render loops
// in the render-time state adjustment pattern.
const emptyOrgs: TypesGen.Organization[] = [];

const createOrgMemberCheck = {
	object: { resource_type: "organization_member" },
	action: "create",
} as const;

export const CreateUserForm: FC<CreateUserFormProps> = (props) => {
	const availableLoginTypes = (
		["password", "oidc", "github", "none"] as const
	).filter((key) => {
		if (key === "none") {
			return props.serviceAccountsEnabled;
		}
		return props.authMethods[key].enabled;
	});
	const defaultLoginType = availableLoginTypes[0];

	if (!defaultLoginType) {
		return (
			<div>
				<Button variant="subtle" asChild className="-ml-3">
					<Link to="/deployment/users">
						<ArrowLeftIcon />
						<span>Back to users</span>
					</Link>
				</Button>
				<div className="pt-6">
					<SettingsHeader>
						<SettingsHeaderTitle>New user</SettingsHeaderTitle>
						<SettingsHeaderDescription>
							Add a user to this Coder deployment.
						</SettingsHeaderDescription>
					</SettingsHeader>
					<Alert severity="error" prominent>
						No authentication methods are available for new users.
					</Alert>
				</div>
			</div>
		);
	}

	return (
		<CreateUserFormFields
			{...props}
			availableLoginTypes={availableLoginTypes}
			defaultLoginType={defaultLoginType}
		/>
	);
};

type AvailableLoginType = keyof typeof loginTypeOptions;

type CreateUserFormFieldsProps = CreateUserFormProps & {
	availableLoginTypes: readonly AvailableLoginType[];
	defaultLoginType: AvailableLoginType;
};

const CreateUserFormFields: FC<CreateUserFormFieldsProps> = ({
	error,
	isLoading,
	onSubmit,
	onCancel,
	showOrganizations,
	availableRoles,
	rolesLoading,
	rolesError,
	availableLoginTypes,
	defaultLoginType,
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
			login_type: defaultLoginType,
			service_account: defaultLoginType === "none",
			roles: new Set(),
		},
		validationSchema,
		onSubmit,
	});

	const [selectedOrg, setSelectedOrg] = useState<TypesGen.Organization | null>(
		null,
	);

	const permittedOrgsQuery = useQuery({
		...permittedOrganizations(createOrgMemberCheck),
		enabled: showOrganizations,
	});
	const orgOptions = permittedOrgsQuery.data ?? emptyOrgs;

	// Clear invalid selections when permission filtering removes the
	// selected org. Uses the React render-time adjustment pattern.
	const [prevOrgOptions, setPrevOrgOptions] = useState(orgOptions);
	if (orgOptions !== prevOrgOptions) {
		setPrevOrgOptions(orgOptions);
		if (selectedOrg && !orgOptions.some((o) => o.id === selectedOrg.id)) {
			setSelectedOrg(null);
			void form.setFieldValue("organization", "");
		}
	}

	// Auto-select when exactly one org is available and nothing is
	// selected. Runs every render (not gated on options change) so it
	// works when mock data is available synchronously on first render.
	if (orgOptions.length === 1 && selectedOrg === null) {
		setSelectedOrg(orgOptions[0]);
		void form.setFieldValue("organization", orgOptions[0].id ?? "");
	}

	const getFieldHelpers = getFormHelpers(form, error);

	const isServiceAccount = form.values.login_type === "none";
	const isPasswordLogin = form.values.login_type === "password";
	const loginTypeField = getFieldHelpers("login_type", {
		helperText: "Authentication method for this user.",
	});

	return (
		<div>
			<Button variant="subtle" asChild className="-ml-3">
				<Link to="/deployment/users">
					<ArrowLeftIcon />
					<span>Back to users</span>
				</Link>
			</Button>
			<div className="pt-6">
				<SettingsHeader>
					<SettingsHeaderTitle>New user</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Add a user to this Coder deployment.
					</SettingsHeaderDescription>
				</SettingsHeader>
				<div className="border border-solid p-6 rounded-lg">
					<form
						onSubmit={form.handleSubmit}
						autoComplete="off"
						className="flex flex-col gap-6"
					>
						{isApiError(error) && !hasApiFieldErrors(error) && (
							<ErrorAlert error={error} />
						)}
						<FormFields>
							{showOrganizations && (
								<div className="flex flex-col gap-2">
									<Label htmlFor="organization">Organization</Label>
									<OrganizationAutocomplete
										id="organization"
										required
										value={selectedOrg}
										options={orgOptions}
										onChange={(newValue) => {
											setSelectedOrg(newValue);
											void form.setFieldValue(
												"organization",
												newValue?.id ?? "",
											);
										}}
									/>
								</div>
							)}

							<div className="flex flex-col gap-2">
								<Label htmlFor="login_type">Login type</Label>
								<Select
									value={form.values.login_type}
									onValueChange={async (value) => {
										const isServiceAccount = value === "none";
										await Promise.all([
											form.setFieldValue("login_type", value),
											form.setFieldValue("service_account", isServiceAccount),
											isServiceAccount
												? form.setFieldValue("email", "")
												: Promise.resolve(),
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
										{availableLoginTypes.map((key) => {
											const opt = loginTypeOptions[key];
											return (
												<SelectPrimitive.Item
													key={key}
													value={key}
													className="relative flex w-full cursor-default select-none items-start rounded-sm py-1.5 pl-2 pr-8 text-sm text-content-secondary outline-hidden focus:bg-surface-secondary focus:text-content-primary data-disabled:pointer-events-none data-disabled:opacity-50"
												>
													<span className="absolute right-2 top-2 flex items-center justify-center">
														<SelectPrimitive.ItemIndicator>
															<CheckIcon className="size-icon-sm" />
														</SelectPrimitive.ItemIndicator>
													</span>
													<div className="flex flex-col py-0.5">
														<SelectPrimitive.ItemText>
															{opt.label}
														</SelectPrimitive.ItemText>
														<span className="text-xs text-content-secondary whitespace-normal wrap-break-word">
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

							<FormField
								field={getFieldHelpers("username", {
									helperText: "Unique identifier.",
								})}
								label="Username"
								required
								onChange={onChangeTrimmed(form)}
								autoComplete="username"
								autoFocus
							/>

							<FormField
								field={getFieldHelpers("name", {
									helperText:
										"Friendly name. Defaults to the username if blank.",
								})}
								label="Name"
								autoComplete="name"
							/>

							{!isServiceAccount && (
								<FormField
									field={getFieldHelpers("email")}
									label="Email"
									required
									autoComplete="email"
									type="email"
								/>
							)}

							{isPasswordLogin && (
								<FormField
									field={getFieldHelpers("password")}
									label="Password"
									required
									autoComplete="new-password"
									type="password"
									// The login type select's visible value is also "Password".
									data-testid="password-input"
								/>
							)}

							<RoleSelector
								loading={rolesLoading}
								error={rolesError}
								availableRoles={availableRoles}
								selectedRoles={form.values.roles}
								onChange={(roles) => form.setFieldValue("roles", roles)}
							/>
						</FormFields>

						<FormFooter className="mt-0">
							<Button onClick={onCancel} variant="outline">
								Cancel
							</Button>
							<Button type="submit" disabled={isLoading}>
								<Spinner loading={isLoading} />
								Save
							</Button>
						</FormFooter>
					</form>
				</div>
			</div>
		</div>
	);
};
