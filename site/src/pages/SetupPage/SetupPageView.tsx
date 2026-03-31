import { isAxiosError } from "axios";
import { type FormikContextType, useFormik } from "formik";
import { type ChangeEvent, type FC, type ReactNode, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import * as Yup from "yup";
import { API } from "#/api/api";
import { countries } from "#/api/countriesGenerated";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { CoderIcon } from "#/components/Icons/CoderIcon";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { useDebouncedValue } from "#/hooks/debounce";
import { cn } from "#/utils/cn";
import {
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "#/utils/formUtils";

export const Language = {
	emailLabel: "Email",
	passwordLabel: "Password",
	nameLabel: "Full Name",
	usernameLabel: "Username",
	emailInvalid: "Please enter a valid email address.",
	emailRequired: "Please enter an email address.",
	passwordRequired: "Please enter a password.",
	create: "Continue with email",
	githubCreate: "Continue with GitHub",
	welcomeMessage: <>Welcome to Coder</>,
	firstNameLabel: "First name",
	lastNameLabel: "Last name",
	companyLabel: "Company",
	jobTitleLabel: "Job title",
	phoneNumberLabel: "Phone number",
	countryLabel: "Country",
	developersLabel: "Number of developers",
	firstNameRequired: "Please enter your first name.",
	phoneNumberRequired: "Please enter your phone number.",
	jobTitleRequired: "Please enter your job title.",
	companyNameRequired: "Please enter your company name.",
	countryRequired: "Please select your country.",
	developersRequired: "Please select the number of developers in your company.",
	isBusinessLabel: "Is this for business use?",
	industryTypeLabel: "Industry",
	orgSizeLabel: "Organization size",
	newsletterSectionLabel: "Newsletter signup",
	newsletterMarketingLabel: "Marketing updates",
	newsletterMarketingDescription:
		"Latest articles, workshops, events, and announcements",
	newsletterReleasesLabel: "Release & security updates",
	newsletterReleasesDescription: "New releases, patches, security advisories",
};

const usernameValidator = nameValidator(Language.usernameLabel);
const usernameFromEmail = (email: string): string => {
	try {
		const emailPrefix = email.split("@")[0];
		const username = emailPrefix.toLowerCase().replace(/[^a-z0-9]/g, "-");
		usernameValidator.validateSync(username);
		return username;
	} catch (error) {
		console.warn(
			"failed to automatically generate username, defaulting to 'admin'",
			error,
		);
		return "admin";
	}
};

const validationSchema = Yup.object({
	email: Yup.string()
		.trim()
		.email(Language.emailInvalid)
		.required(Language.emailRequired),
	password: Yup.string().required(Language.passwordRequired),
	username: usernameValidator,
	trial: Yup.bool(),
	trial_info: Yup.object().when("trial", {
		is: true,
		then: (schema) =>
			schema.shape({
				first_name: Yup.string().required(Language.firstNameRequired),
				last_name: Yup.string().required(Language.firstNameRequired),
				phone_number: Yup.string().required(Language.phoneNumberRequired),
				job_title: Yup.string().required(Language.jobTitleRequired),
				company_name: Yup.string().required(Language.companyNameRequired),
				country: Yup.string().required(Language.countryRequired),
			}),
	}),
	onboarding_info: Yup.object().shape({
		is_business: Yup.bool(),
		industry_type: Yup.string(),
		org_size: Yup.string(),
		newsletter_marketing: Yup.bool(),
		newsletter_releases: Yup.bool(),
	}),
});

const industryTypeOptions = [
	"Technology",
	"Financial Services",
	"Healthcare",
	"Government",
	"Education",
	"Retail",
	"Manufacturing",
	"Media",
	"Telecom",
	"Energy",
	"Transportation",
	"Consulting",
	"Non-Profit",
	"Other",
];

const orgSizeOptions = [
	"Just me",
	"2-10",
	"11-50",
	"51-200",
	"201-1000",
	"1001-5000",
	"5000+",
];

interface SetupPageViewProps {
	onSubmit: (firstUser: TypesGen.CreateFirstUserRequest) => void;
	error?: unknown;
	isLoading?: boolean;
	authMethods: TypesGen.AuthMethods | undefined;
}

// Reusable field wrapper matching the shadcn/ui pattern used across Coder.
const Field: FC<{
	label: string;
	id: string;
	error?: boolean;
	helperText?: ReactNode;
	className?: string;
	children: React.ReactNode;
}> = ({ label, id, error, helperText, className, children }) => (
	<div className={cn("flex flex-col items-start gap-1", className)}>
		<Label htmlFor={id} className="text-sm font-medium">
			{label}
		</Label>
		{children}
		{helperText && (
			<span
				className={cn(
					"text-xs text-left",
					error ? "text-content-destructive" : "text-content-secondary",
				)}
			>
				{helperText}
			</span>
		)}
	</div>
);

export const SetupPageView: FC<SetupPageViewProps> = ({
	onSubmit,
	error,
	isLoading,
	authMethods,
}) => {
	// Track the is_business select display value separately because the form
	// stores a boolean but we need three visual states: unselected / yes / no.
	const [isBusinessDisplay, setIsBusinessDisplay] = useState("");

	const form: FormikContextType<TypesGen.CreateFirstUserRequest> =
		useFormik<TypesGen.CreateFirstUserRequest>({
			initialValues: {
				email: "",
				password: "",
				username: "",
				name: "",
				trial: false,
				trial_info: {
					first_name: "",
					last_name: "",
					phone_number: "",
					job_title: "",
					company_name: "",
					country: "",
					developers: "",
				},
				onboarding_info: {
					is_business: false,
					industry_type: "",
					org_size: "",
					newsletter_marketing: false,
					newsletter_releases: false,
				},
			},
			validationSchema,
			onSubmit,
			validateOnBlur: false,
		});
	const getFieldHelpers = getFormHelpers<TypesGen.CreateFirstUserRequest>(
		form,
		error,
	);

	// Debounced server-side password validation to match the old PasswordField
	// behavior while using the new Input component.
	const debouncedPassword = useDebouncedValue(form.values.password, 500);
	const validatePasswordQuery = useQuery({
		queryKey: ["validatePassword", debouncedPassword],
		queryFn: () => API.validateUserPassword(debouncedPassword),
		placeholderData: keepPreviousData,
		enabled: debouncedPassword.length > 0,
	});
	const passwordValid = validatePasswordQuery.data?.valid ?? true;

	const emailField = getFieldHelpers("email");
	const passwordField = getFieldHelpers("password");

	return (
		<div className="grow basis-0 min-h-screen flex justify-center items-center py-12">
			<div className="flex flex-col w-full max-w-[500px] px-4">
				<header className="mb-8">
					<CoderIcon className="w-12 h-12 text-content-primary" />
					<h1 className="text-2xl font-normal mt-4 mb-0">
						Welcome to <strong>Coder</strong>
					</h1>
					<p className="mt-3 mb-0 text-sm text-content-secondary">
						Set up your admin account and start building secure, reproducible
						dev environments.
					</p>
				</header>

				<form onSubmit={form.handleSubmit} className="flex flex-col gap-6">
					{authMethods?.github.enabled && (
						<>
							<Button className="w-full" asChild type="submit" size="lg">
								<a href="/api/v2/users/oauth2/github/callback">
									<ExternalImage src="/icon/github.svg" className="invert" />
									{Language.githubCreate}
								</a>
							</Button>
							<div className="flex items-center gap-4">
								<div className="h-[1px] w-full bg-border" />
								<div className="shrink-0 text-xs uppercase text-content-secondary tracking-wider">
									or
								</div>
								<div className="h-[1px] w-full bg-border" />
							</div>
						</>
					)}

					{/* Email */}
					<Field
						label={Language.emailLabel}
						id="email"
						error={emailField.error}
						helperText={emailField.helperText}
					>
						<Input
							id="email"
							name="email"
							value={emailField.value}
							onChange={(event) => {
								const email = event.target.value;
								const username = usernameFromEmail(email);
								form.setFieldValue("username", username);
								onChangeTrimmed(form)(event as ChangeEvent<HTMLInputElement>);
							}}
							onBlur={emailField.onBlur}
							autoComplete="email"
							aria-invalid={emailField.error}
						/>
					</Field>

					{/* Password */}
					<Field
						label={Language.passwordLabel}
						id="password"
						error={!passwordValid || passwordField.error}
						helperText={
							!passwordValid
								? validatePasswordQuery.data?.details
								: passwordField.helperText
						}
					>
						<Input
							id="password"
							name="password"
							type="password"
							value={passwordField.value}
							onChange={form.handleChange}
							onBlur={passwordField.onBlur}
							autoComplete="current-password"
							aria-invalid={!passwordValid || passwordField.error}
						/>
					</Field>

					{/* Enterprise trial toggle */}
					<label
						htmlFor="trial"
						className="flex cursor-pointer gap-2 items-start"
					>
						<Checkbox
							id="trial"
							name="trial"
							checked={form.values.trial}
							onCheckedChange={(checked) =>
								form.setFieldValue("trial", checked === true)
							}
							data-testid="trial"
							className="mt-0.5"
						/>
						<div className="flex flex-col gap-0.5">
							<span className="text-sm font-semibold">
								Start a 30-day trial of Enterprise
							</span>
							<span className="text-xs text-content-secondary leading-relaxed">
								Get access to high availability, template RBAC, audit logging,
								quotas, and more.
							</span>
							<a
								href="https://coder.com/pricing"
								target="_blank"
								rel="noreferrer"
								className="text-xs text-content-link hover:underline mt-0.5"
							>
								Learn more
							</a>
						</div>
					</label>

					{/* Conditional trial info fields */}
					{form.values.trial && (
						<div className="flex flex-col gap-4">
							<div className="grid grid-cols-2 gap-3">
								<Field
									label={Language.firstNameLabel}
									id="trial_info.first_name"
									error={getFieldHelpers("trial_info.first_name").error}
									helperText={
										getFieldHelpers("trial_info.first_name").helperText
									}
								>
									<Input
										id="trial_info.first_name"
										name="trial_info.first_name"
										value={form.values.trial_info.first_name}
										onChange={form.handleChange}
										onBlur={form.handleBlur}
									/>
								</Field>
								<Field
									label={Language.lastNameLabel}
									id="trial_info.last_name"
									error={getFieldHelpers("trial_info.last_name").error}
									helperText={
										getFieldHelpers("trial_info.last_name").helperText
									}
								>
									<Input
										id="trial_info.last_name"
										name="trial_info.last_name"
										value={form.values.trial_info.last_name}
										onChange={form.handleChange}
										onBlur={form.handleBlur}
									/>
								</Field>
							</div>

							<Field
								label={Language.companyLabel}
								id="trial_info.company_name"
								error={getFieldHelpers("trial_info.company_name").error}
								helperText={
									getFieldHelpers("trial_info.company_name").helperText
								}
							>
								<Input
									id="trial_info.company_name"
									name="trial_info.company_name"
									value={form.values.trial_info.company_name}
									onChange={form.handleChange}
									onBlur={form.handleBlur}
								/>
							</Field>

							<Field
								label={Language.jobTitleLabel}
								id="trial_info.job_title"
								error={getFieldHelpers("trial_info.job_title").error}
								helperText={getFieldHelpers("trial_info.job_title").helperText}
							>
								<Input
									id="trial_info.job_title"
									name="trial_info.job_title"
									value={form.values.trial_info.job_title}
									onChange={form.handleChange}
									onBlur={form.handleBlur}
								/>
							</Field>

							<div className="grid grid-cols-2 gap-3">
								<Field
									label={Language.phoneNumberLabel}
									id="trial_info.phone_number"
									error={getFieldHelpers("trial_info.phone_number").error}
									helperText={
										getFieldHelpers("trial_info.phone_number").helperText
									}
								>
									<Input
										id="trial_info.phone_number"
										name="trial_info.phone_number"
										value={form.values.trial_info.phone_number}
										onChange={form.handleChange}
										onBlur={form.handleBlur}
									/>
								</Field>
								<Field
									label={Language.countryLabel}
									id="trial_info.country"
									error={getFieldHelpers("trial_info.country").error}
									helperText={getFieldHelpers("trial_info.country").helperText}
								>
									<Select
										value={form.values.trial_info.country}
										onValueChange={(value) =>
											form.setFieldValue("trial_info.country", value)
										}
									>
										<SelectTrigger id="trial_info.country">
											<SelectValue placeholder="Select..." />
										</SelectTrigger>
										<SelectContent>
											{countries.map((c) => (
												<SelectItem key={c.name} value={c.name}>
													{c.flag} {c.name}
												</SelectItem>
											))}
										</SelectContent>
									</Select>
								</Field>
							</div>
						</div>
					)}

					{/* Divider */}
					<hr className="w-full border-0 border-t border-solid border-border my-2" />
					{/* Onboarding info — always visible, all optional */}
					<div className="flex flex-col gap-4">
						<div>
							<span className="text-sm font-semibold">
								Help us make Coder better
							</span>
							<span className="text-xs text-content-secondary ml-1.5">
								(optional)
							</span>
						</div>

						<div className="w-1/2 pr-1.5">
							<Field
								label={Language.isBusinessLabel}
								id="onboarding_info.is_business"
							>
								<Select
									value={isBusinessDisplay}
									onValueChange={(value) => {
										setIsBusinessDisplay(value);
										form.setFieldValue(
											"onboarding_info.is_business",
											value === "yes",
										);
									}}
								>
									<SelectTrigger
										id="onboarding_info.is_business"
										data-testid="onboarding_info.is_business"
									>
										<SelectValue placeholder="Select..." />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="yes">Yes</SelectItem>
										<SelectItem value="no">No</SelectItem>
									</SelectContent>
								</Select>
							</Field>
						</div>

						{isBusinessDisplay === "yes" && (
							<div className="grid grid-cols-2 gap-3">
								<Field
									label={Language.industryTypeLabel}
									id="onboarding_info.industry_type"
								>
									<Select
										value={form.values.onboarding_info.industry_type}
										onValueChange={(value) =>
											form.setFieldValue("onboarding_info.industry_type", value)
										}
									>
										<SelectTrigger
											id="onboarding_info.industry_type"
											data-testid="onboarding_info.industry_type"
										>
											<SelectValue placeholder="Select..." />
										</SelectTrigger>
										<SelectContent>
											{industryTypeOptions.map((opt) => (
												<SelectItem key={opt} value={opt}>
													{opt}
												</SelectItem>
											))}
										</SelectContent>
									</Select>
								</Field>

								<Field
									label={Language.orgSizeLabel}
									id="onboarding_info.org_size"
								>
									<Select
										value={form.values.onboarding_info.org_size}
										onValueChange={(value) =>
											form.setFieldValue("onboarding_info.org_size", value)
										}
									>
										<SelectTrigger
											id="onboarding_info.org_size"
											data-testid="onboarding_info.org_size"
										>
											<SelectValue placeholder="Select..." />
										</SelectTrigger>
										<SelectContent>
											{orgSizeOptions.map((opt) => (
												<SelectItem key={opt} value={opt}>
													{opt}
												</SelectItem>
											))}
										</SelectContent>
									</Select>
								</Field>
							</div>
						)}
						{/* Newsletter signup */}
						<div className="mt-2">
							<span className="text-sm font-semibold block mb-2">
								{Language.newsletterSectionLabel}
							</span>

							<label
								htmlFor="onboarding_info.newsletter_marketing"
								className="flex cursor-pointer gap-2 items-start mb-2"
							>
								<Checkbox
									id="onboarding_info.newsletter_marketing"
									checked={form.values.onboarding_info.newsletter_marketing}
									onCheckedChange={(checked) =>
										form.setFieldValue(
											"onboarding_info.newsletter_marketing",
											checked === true,
										)
									}
									data-testid="onboarding_info.newsletter_marketing"
									className="mt-0.5"
								/>
								<div>
									<span className="text-sm font-medium block">
										{Language.newsletterMarketingLabel}
									</span>
									<span className="text-xs text-content-secondary block">
										{Language.newsletterMarketingDescription}
									</span>
								</div>
							</label>

							<label
								htmlFor="onboarding_info.newsletter_releases"
								className="flex cursor-pointer gap-2 items-start"
							>
								<Checkbox
									id="onboarding_info.newsletter_releases"
									checked={form.values.onboarding_info.newsletter_releases}
									onCheckedChange={(checked) =>
										form.setFieldValue(
											"onboarding_info.newsletter_releases",
											checked === true,
										)
									}
									data-testid="onboarding_info.newsletter_releases"
									className="mt-0.5"
								/>
								<div>
									<span className="text-sm font-medium block">
										{Language.newsletterReleasesLabel}
									</span>
									<span className="text-xs text-content-secondary block">
										{Language.newsletterReleasesDescription}
									</span>
								</div>
							</label>
						</div>

						{/* Privacy policy notice */}
						<p className="text-xs text-content-secondary leading-relaxed mt-2">
							The information you provide will be treated in accordance with the{" "}
							<a
								href="https://coder.com/legal/privacy-policy"
								target="_blank"
								rel="noreferrer"
								className="text-content-link hover:underline"
							>
								Coder Privacy Policy
							</a>
							. Opt-out at any time.
						</p>
					</div>

					{isAxiosError(error) && error.response?.data?.message && (
						<Alert severity="error" prominent>
							<AlertTitle>{error.response.data.message}</AlertTitle>
							{error.response.data.detail && (
								<AlertDescription>
									{error.response.data.detail}
									<br />
									<a
										target="_blank"
										rel="noreferrer"
										href="https://coder.com/contact/sales"
										className="text-content-link hover:underline"
									>
										Contact Sales
									</a>
								</AlertDescription>
							)}
						</Alert>
					)}

					<Button
						className="w-full"
						disabled={isLoading}
						type="submit"
						data-testid="create"
						size="lg"
					>
						<Spinner loading={isLoading} />
						{Language.create}
					</Button>
				</form>

				<div className="text-xs text-content-secondary pt-6">
					&copy; {new Date().getFullYear()} Coder Technologies, Inc.
				</div>
			</div>
		</div>
	);
};
