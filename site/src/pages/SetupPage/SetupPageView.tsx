import { countries } from "api/countriesGenerated";
import type * as TypesGen from "api/typesGenerated";
import { isAxiosError } from "axios";
import { Alert, AlertDetail, AlertTitle } from "components/Alert/Alert";
import { Autocomplete } from "components/Autocomplete/Autocomplete";
import { Button } from "components/Button/Button";
import { Checkbox } from "components/Checkbox/Checkbox";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { FormFields, VerticalForm } from "components/Form/Form";
import { CoderIcon } from "components/Icons/CoderIcon";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { Link } from "components/Link/Link";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "components/Select/Select";
import { SignInLayout } from "components/SignInLayout/SignInLayout";
import { Spinner } from "components/Spinner/Spinner";
import { type FormikContextType, useFormik } from "formik";
import type { ChangeEvent, FC } from "react";
import {
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "utils/formUtils";
import * as Yup from "yup";

export const Language = {
	emailLabel: "Email",
	passwordLabel: "Password",
	nameLabel: "Full Name",
	usernameLabel: "Username",
	emailInvalid: "Please enter a valid email address.",
	emailRequired: "Please enter an email address.",
	passwordRequired: "Please enter a password.",
	create: "Continue with email",
	githubCreate: "GitHub",
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
				developers: Yup.string().required(Language.developersRequired),
			}),
	}),
});

const numberOfDevelopersOptions = [
	"1-100",
	"101-500",
	"501-1000",
	"1001-2500",
	"2500+",
];

interface SetupPageViewProps {
	onSubmit: (firstUser: TypesGen.CreateFirstUserRequest) => void;
	error?: unknown;
	isLoading?: boolean;
	authMethods: TypesGen.AuthMethods | undefined;
}

export const SetupPageView: FC<SetupPageViewProps> = ({
	onSubmit,
	error,
	isLoading,
	authMethods,
}) => {
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
			},
			validationSchema,
			onSubmit,
			// With validate on blur set to true, the form lights up red whenever
			// you click out of it. This is a bit jarring. We instead validate
			// on submit and change.
			validateOnBlur: false,
		});
	const getFieldHelpers = getFormHelpers<TypesGen.CreateFirstUserRequest>(
		form,
		error,
	);

	const emailField = getFieldHelpers("email");
	const passwordField = getFieldHelpers("password");
	const firstNameField = getFieldHelpers("trial_info.first_name");
	const lastNameField = getFieldHelpers("trial_info.last_name");
	const companyNameField = getFieldHelpers("trial_info.company_name");
	const jobTitleField = getFieldHelpers("trial_info.job_title");
	const phoneNumberField = getFieldHelpers("trial_info.phone_number");
	const countryField = getFieldHelpers("trial_info.country");
	const developersField = getFieldHelpers("trial_info.developers");
	const selectedCountry =
		countries.find(
			(country) => country.name === form.values.trial_info.country,
		) ?? null;

	return (
		<SignInLayout>
			<header className="mb-8 text-center">
				<CoderIcon className="w-12 h-12" />
				<h1 className="m-0 mt-4 font-semibold">Welcome to Coder</h1>
				<div className="mt-3 text-content-secondary">
					Let&lsquo;s create your first admin user account
				</div>
			</header>
			<VerticalForm onSubmit={form.handleSubmit}>
				<FormFields>
					{authMethods?.github.enabled && (
						<>
							<Button
								className="w-full"
								asChild
								type="submit"
								size="lg"
								variant="outline"
							>
								<a href="/api/v2/users/oauth2/github/callback">
									<ExternalImage src="/icon/github.svg" />
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
					<div className="flex flex-col items-start gap-2">
						<Label htmlFor={emailField.id}>{Language.emailLabel}</Label>
						<Input
							id={emailField.id}
							name={emailField.name}
							value={emailField.value}
							onBlur={emailField.onBlur}
							onChange={(event) => {
								const email = event.target.value;
								const username = usernameFromEmail(email);
								form.setFieldValue("username", username);
								onChangeTrimmed(form)(event as ChangeEvent<HTMLInputElement>);
							}}
							autoComplete="email"
							type="email"
							aria-invalid={emailField.error}
						/>
						{emailField.error && (
							<span className="text-xs text-content-destructive text-left">
								{emailField.helperText}
							</span>
						)}
					</div>
					<div className="flex flex-col items-start gap-2">
						<Label htmlFor={passwordField.id}>{Language.passwordLabel}</Label>
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
							<span className="text-xs text-content-destructive text-left">
								{passwordField.helperText}
							</span>
						)}
					</div>
					<label
						htmlFor="trial"
						className="-mt-1 mb-2 flex cursor-pointer items-start gap-3"
					>
						<div>
							<Checkbox
								id="trial"
								name="trial"
								checked={form.values.trial}
								onCheckedChange={(checked) => {
									void form.setFieldValue("trial", Boolean(checked));
								}}
								data-testid="trial"
							/>
						</div>

						<div className="flex flex-col items-start text-sm pt-0.5">
							<span>Start a free trial of Enterprise</span>
							<span className="text-content-secondary">
								Get access to high availability, template RBAC, audit logging,
								quotas, and more.
							</span>
							<Link
								href="https://coder.com/pricing"
								target="_blank"
								className="mt-0.5 p-0 inline-flex items-center"
							>
								Read more
							</Link>
						</div>
					</label>

					{form.values.trial && (
						<>
							<div className="flex gap-3">
								<div className="flex-1 flex flex-col items-start gap-2">
									<Label htmlFor={firstNameField.id}>
										{Language.firstNameLabel}
									</Label>
									<Input
										id={firstNameField.id}
										name={firstNameField.name}
										value={firstNameField.value}
										onChange={firstNameField.onChange}
										onBlur={firstNameField.onBlur}
										aria-invalid={firstNameField.error}
									/>
									{firstNameField.error && (
										<span className="text-xs text-content-destructive text-left">
											{firstNameField.helperText}
										</span>
									)}
								</div>
								<div className="flex-1 flex flex-col items-start gap-2">
									<Label htmlFor={lastNameField.id}>
										{Language.lastNameLabel}
									</Label>
									<Input
										id={lastNameField.id}
										name={lastNameField.name}
										value={lastNameField.value}
										onChange={lastNameField.onChange}
										onBlur={lastNameField.onBlur}
										aria-invalid={lastNameField.error}
									/>
									{lastNameField.error && (
										<span className="text-xs text-content-destructive text-left">
											{lastNameField.helperText}
										</span>
									)}
								</div>
							</div>
							<div className="flex flex-col items-start gap-2">
								<Label htmlFor={companyNameField.id}>
									{Language.companyLabel}
								</Label>
								<Input
									id={companyNameField.id}
									name={companyNameField.name}
									value={companyNameField.value}
									onChange={companyNameField.onChange}
									onBlur={companyNameField.onBlur}
									aria-invalid={companyNameField.error}
								/>
								{companyNameField.error && (
									<span className="text-xs text-content-destructive text-left">
										{companyNameField.helperText}
									</span>
								)}
							</div>
							<div className="flex flex-col items-start gap-2">
								<Label htmlFor={jobTitleField.id}>
									{Language.jobTitleLabel}
								</Label>
								<Input
									id={jobTitleField.id}
									name={jobTitleField.name}
									value={jobTitleField.value}
									onChange={jobTitleField.onChange}
									onBlur={jobTitleField.onBlur}
									aria-invalid={jobTitleField.error}
								/>
								{jobTitleField.error && (
									<span className="text-xs text-content-destructive text-left">
										{jobTitleField.helperText}
									</span>
								)}
							</div>
							<div className="flex flex-col items-start gap-2">
								<Label htmlFor={phoneNumberField.id}>
									{Language.phoneNumberLabel}
								</Label>
								<Input
									id={phoneNumberField.id}
									name={phoneNumberField.name}
									value={phoneNumberField.value}
									onChange={phoneNumberField.onChange}
									onBlur={phoneNumberField.onBlur}
									aria-invalid={phoneNumberField.error}
								/>
								{phoneNumberField.error && (
									<span className="text-xs text-content-destructive text-left">
										{phoneNumberField.helperText}
									</span>
								)}
							</div>
							<div className="flex flex-col items-start gap-2">
								<Label htmlFor={countryField.id}>{Language.countryLabel}</Label>
								<Autocomplete
									id={countryField.id}
									options={countries}
									value={selectedCountry}
									getOptionLabel={(option) => `${option.flag} ${option.name}`}
									getOptionValue={(option) => option.name}
									onChange={(newValue) => {
										void form.setFieldValue(
											"trial_info.country",
											newValue?.name ?? "",
										);
									}}
								/>
								{countryField.error && (
									<span className="text-xs text-content-destructive text-left">
										{countryField.helperText}
									</span>
								)}
							</div>
							<div className="flex flex-col items-start gap-2">
								<Label htmlFor={developersField.id}>
									{Language.developersLabel}
								</Label>
								<Select
									value={String(developersField.value ?? "")}
									onValueChange={(value) => {
										void form.setFieldValue("trial_info.developers", value);
									}}
								>
									<SelectTrigger id={developersField.id}>
										<SelectValue placeholder={Language.developersLabel} />
									</SelectTrigger>
									<SelectContent>
										{numberOfDevelopersOptions.map((opt) => (
											<SelectItem key={opt} value={opt}>
												{opt}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
								{developersField.error && (
									<span className="text-xs text-content-destructive text-left">
										{developersField.helperText}
									</span>
								)}
							</div>
							<div className="-mt-1 text-center text-2xs text-content-secondary">
								Complete the form to receive your trial license and be contacted
								about Coder products and solutions. The information you provide
								will be treated in accordance with the{" "}
								<Link
									href="https://coder.com/legal/privacy-policy"
									target="_blank"
									className="text-2xs px-0"
									showExternalIcon={false}
								>
									Coder Privacy Policy
								</Link>
								. Opt-out at any time.
							</div>
						</>
					)}

					{isAxiosError(error) && error.response?.data?.message && (
						<Alert severity="error" prominent>
							<AlertTitle>{error.response.data.message}</AlertTitle>
							{error.response.data.detail && (
								<AlertDetail>
									{error.response.data.detail}
									<br />
									<Link target="_blank" href="https://coder.com/contact/sales">
										Contact Sales
									</Link>
								</AlertDetail>
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
				</FormFields>
			</VerticalForm>
		</SignInLayout>
	);
};
