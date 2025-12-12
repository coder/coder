import AlertTitle from "@mui/material/AlertTitle";
import Autocomplete from "@mui/material/Autocomplete";
import Checkbox from "@mui/material/Checkbox";
import Link from "@mui/material/Link";
import MenuItem from "@mui/material/MenuItem";
import TextField from "@mui/material/TextField";
import { countries } from "api/countriesGenerated";
import type * as TypesGen from "api/typesGenerated";
import { isAxiosError } from "axios";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { FormFields, VerticalForm } from "components/Form/Form";
import { CoderIcon } from "components/Icons/CoderIcon";
import { PasswordField } from "components/PasswordField/PasswordField";
import { SignInLayout } from "components/SignInLayout/SignInLayout";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
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

	return (
		<SignInLayout>
			<header className="text-center mb-8">
				<CoderIcon className="w-12 h-12" />
				<h1 className="font-normal m-0 mt-4">
					Welcome to <strong>Coder</strong>
				</h1>
				<div
					css={(theme) => ({
						color: theme.palette.text.secondary,
					})}
					className="mt-3"
				>
					Let&lsquo;s create your first admin user account
				</div>
			</header>
			<VerticalForm onSubmit={form.handleSubmit}>
				<FormFields>
					{authMethods?.github.enabled && (
						<>
							<Button className="w-full" asChild type="submit" size="lg">
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
					<TextField
						{...getFieldHelpers("email")}
						onChange={(event) => {
							const email = event.target.value;
							const username = usernameFromEmail(email);
							form.setFieldValue("username", username);
							onChangeTrimmed(form)(event as ChangeEvent<HTMLInputElement>);
						}}
						autoComplete="email"
						fullWidth
						label={Language.emailLabel}
					/>
					<PasswordField
						{...getFieldHelpers("password")}
						autoComplete="current-password"
						fullWidth
						label={Language.passwordLabel}
					/>
					<label
						htmlFor="trial"
						className="flex cursor-pointer items-start gap-1 -mt-1 mb-2"
					>
						<Checkbox
							id="trial"
							name="trial"
							checked={form.values.trial}
							onChange={form.handleChange}
							data-testid="trial"
							size="small"
						/>

						<div className="text-sm leading-loose pt-1">
							<span className="block font-semibold">
								Start a free trial of Enterprise
							</span>
							<span
								css={(theme) => ({
									color: theme.palette.text.secondary,
								})}
								className="block text-[13px] leading-relaxed"
							>
								Get access to high availability, template RBAC, audit logging,
								quotas, and more.
							</span>
							<Link
								href="https://coder.com/pricing"
								target="_blank"
								className="mt-1 text-[13px] inline-block"
							>
								Read more
							</Link>
						</div>
					</label>

					{form.values.trial && (
						<>
							<Stack spacing={1.5} direction="row">
								<TextField
									{...getFieldHelpers("trial_info.first_name")}
									id="trial_info.first_name"
									name="trial_info.first_name"
									fullWidth
									label={Language.firstNameLabel}
								/>
								<TextField
									{...getFieldHelpers("trial_info.last_name")}
									id="trial_info.last_name"
									name="trial_info.last_name"
									fullWidth
									label={Language.lastNameLabel}
								/>
							</Stack>
							<TextField
								{...getFieldHelpers("trial_info.company_name")}
								id="trial_info.company_name"
								name="trial_info.company_name"
								fullWidth
								label={Language.companyLabel}
							/>
							<TextField
								{...getFieldHelpers("trial_info.job_title")}
								id="trial_info.job_title"
								name="trial_info.job_title"
								fullWidth
								label={Language.jobTitleLabel}
							/>
							<TextField
								{...getFieldHelpers("trial_info.phone_number")}
								id="trial_info.phone_number"
								name="trial_info.phone_number"
								fullWidth
								label={Language.phoneNumberLabel}
							/>
							<Autocomplete
								autoHighlight
								options={countries}
								renderOption={(props, country) => (
									<li {...props}>{`${country.flag} ${country.name}`}</li>
								)}
								getOptionLabel={(option) => option.name}
								onChange={(_, newValue) =>
									form.setFieldValue("trial_info.country", newValue?.name)
								}
								css={{
									"&:not(:has(label))": {
										margin: 0,
									},
								}}
								renderInput={(params) => (
									<TextField
										{...params}
										{...getFieldHelpers("trial_info.country")}
										id="trial_info.country"
										name="trial_info.country"
										label={Language.countryLabel}
										fullWidth
										inputProps={{
											...params.inputProps,
										}}
										InputLabelProps={{ shrink: true }}
									/>
								)}
							/>
							<TextField
								{...getFieldHelpers("trial_info.developers")}
								id="trial_info.developers"
								name="trial_info.developers"
								fullWidth
								label={Language.developersLabel}
								select
							>
								{numberOfDevelopersOptions.map((opt) => (
									<MenuItem key={opt} value={opt}>
										{opt}
									</MenuItem>
								))}
							</TextField>
							<div
								css={(theme) => ({
									color: theme.palette.text.secondary,
								})}
								className="text-[11px] text-center mt-[-5px] leading-normal"
							>
								Complete the form to receive your trial license and be contacted
								about Coder products and solutions. The information you provide
								will be treated in accordance with the{" "}
								<Link
									href="https://coder.com/legal/privacy-policy"
									target="_blank"
								>
									Coder Privacy Policy
								</Link>
								. Opt-out at any time.
							</div>
						</>
					)}

					{isAxiosError(error) && error.response?.data?.message && (
						<Alert severity="error">
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
