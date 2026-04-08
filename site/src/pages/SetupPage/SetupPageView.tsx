import { isAxiosError } from "axios";
import { type FormikContextType, useFormik } from "formik";
import type { FC, ReactNode } from "react";
import * as Yup from "yup";
import { countries } from "#/api/countriesGenerated";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { FormField } from "#/components/FormField/FormField";
import { CoderIcon } from "#/components/Icons/CoderIcon";
import { Label } from "#/components/Label/Label";
import { PasswordField } from "#/components/PasswordField/PasswordField";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { cn } from "#/utils/cn";
import {
	type FormHelpers,
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "#/utils/formUtils";

const usernameValidator = nameValidator("Username");
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
		.email("Please enter a valid email address.")
		.required("Please enter an email address."),
	password: Yup.string().required("Please enter a password."),
	username: usernameValidator,
	trial: Yup.bool(),
	trial_info: Yup.object().when("trial", {
		is: true,
		then: (schema) =>
			schema.shape({
				first_name: Yup.string().required("Please enter your first name."),
				last_name: Yup.string().required("Please enter your last name."),
				phone_number: Yup.string().required("Please enter your phone number."),
				job_title: Yup.string().required("Please enter your job title."),
				company_name: Yup.string().required("Please enter your company name."),
				country: Yup.string().required("Please select your country."),
				developers: Yup.string().required(
					"Please select the number of developers in your company.",
				),
			}),
	}),
	onboarding_info: Yup.object().shape({
		newsletter_marketing: Yup.bool(),
		newsletter_releases: Yup.bool(),
	}),
});

const numberOfDevelopersOptions = [
	"1-100",
	"101-500",
	"501-1000",
	"1001-2500",
	"2500+",
];

const Field: FC<{
	label: string;
	id: string;
	error?: boolean;
	helperText?: ReactNode;
	className?: string;
	children: ReactNode;
}> = ({ label, id, error, helperText, className, children }) => (
	<div className={cn("flex flex-col gap-2", className)}>
		<Label htmlFor={id}>{label}</Label>
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

type SelectFieldProps = FormHelpers & {
	label: string;
	className?: string;
	onValueChange: (value: string) => void;
	placeholder?: string;
	children: ReactNode;
};

const SelectField: FC<SelectFieldProps> = ({
	label,
	id,
	error,
	helperText,
	className,
	value,
	onValueChange,
	placeholder,
	children,
}) => (
	<Field
		label={label}
		id={id}
		error={error}
		helperText={helperText}
		className={className}
	>
		<Select value={String(value ?? "")} onValueChange={onValueChange}>
			<SelectTrigger id={id}>
				<SelectValue placeholder={placeholder} />
			</SelectTrigger>
			<SelectContent>{children}</SelectContent>
		</Select>
	</Field>
);

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
				onboarding_info: {
					newsletter_marketing: false,
					newsletter_releases: false,
				},
			},
			validationSchema,
			onSubmit,
			validateOnBlur: false,
			validateOnMount: true,
		});
	const getFieldHelpers = getFormHelpers<TypesGen.CreateFirstUserRequest>(
		form,
		error,
	);

	return (
		<div className="grow basis-0 min-h-screen flex justify-center items-center py-12">
			<div className="flex flex-col w-full max-w-[500px] px-4">
				<header className="mb-8">
					<CoderIcon className="w-12 h-12 text-content-primary" />
					<h1 className="text-2xl font-normal mt-4 mb-0">
						Welcome to <strong>Coder</strong>
					</h1>
					<p className="mt-3 mb-0 text-sm text-content-secondary font-normal">
						Set up your admin account and start building secure, reproducible
						dev environments.
					</p>
				</header>

				<form onSubmit={form.handleSubmit} className="flex flex-col gap-6">
					{authMethods?.github.enabled && (
						<>
							<Button className="w-full" asChild type="submit" size="lg">
								<a href="/api/v2/users/oauth2/github/callback">
									<ExternalImage src="/icon/github.svg?blackWithColor" />
									GitHub
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
					<FormField
						label="Email"
						field={getFieldHelpers("email")}
						autoComplete="email"
						onChange={onChangeTrimmed(form, (email) => {
							form.setFieldValue("username", usernameFromEmail(email));
						})}
					/>

					{/* Password */}
					<PasswordField
						field={getFieldHelpers("password")}
						label="Password"
						autoComplete="new-password"
					/>

					{/* Premium trial toggle */}
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
						<div className="flex flex-col items-start gap-0.5">
							<span className="text-sm font-semibold">
								Start a 30-day trial of Premium
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
								<FormField
									label="First name"
									field={getFieldHelpers("trial_info.first_name")}
								/>
								<FormField
									label="Last name"
									field={getFieldHelpers("trial_info.last_name")}
								/>
							</div>

							<div className="grid grid-cols-2 gap-3">
								<FormField
									label="Company"
									field={getFieldHelpers("trial_info.company_name")}
								/>
								<SelectField
									label="Number of developers"
									{...getFieldHelpers("trial_info.developers")}
									onValueChange={(value: string) =>
										form.setFieldValue("trial_info.developers", value)
									}
									placeholder="Select..."
								>
									{numberOfDevelopersOptions.map((opt) => (
										<SelectItem key={opt} value={opt}>
											{opt}
										</SelectItem>
									))}
								</SelectField>
							</div>
							<FormField
								label="Job title"
								field={getFieldHelpers("trial_info.job_title")}
							/>

							<div className="grid grid-cols-2 gap-3">
								<FormField
									label="Phone number"
									field={getFieldHelpers("trial_info.phone_number")}
								/>
								<SelectField
									label="Country"
									{...getFieldHelpers("trial_info.country")}
									onValueChange={(value: string) =>
										form.setFieldValue("trial_info.country", value)
									}
									placeholder="Select..."
								>
									{countries.map((c) => (
										<SelectItem key={c.name} value={c.name}>
											{c.flag} {c.name}
										</SelectItem>
									))}
								</SelectField>
							</div>
						</div>
					)}

					{/* Sign up for updates */}
					<div className="flex flex-col gap-3">
						<span className="text-sm font-semibold">Sign up for updates</span>

						<label
							htmlFor="onboarding_info.newsletter_releases"
							className="flex cursor-pointer gap-2 items-start"
						>
							<Checkbox
								id="onboarding_info.newsletter_releases"
								checked={
									form.values.onboarding_info?.newsletter_releases ?? false
								}
								onCheckedChange={(checked) =>
									form.setFieldValue(
										"onboarding_info.newsletter_releases",
										checked === true,
									)
								}
								data-testid="onboarding_info.newsletter_releases"
							/>
							<div className="flex flex-col text-sm">
								<span className="font-medium">Release notes & updates</span>
								<span className="text-content-secondary">
									Monthly changelog and security notices
								</span>
							</div>
						</label>

						<label
							htmlFor="onboarding_info.newsletter_marketing"
							className="flex cursor-pointer gap-2 items-start"
						>
							<Checkbox
								id="onboarding_info.newsletter_marketing"
								checked={
									form.values.onboarding_info?.newsletter_marketing ?? false
								}
								onCheckedChange={(checked) =>
									form.setFieldValue(
										"onboarding_info.newsletter_marketing",
										checked === true,
									)
								}
								data-testid="onboarding_info.newsletter_marketing"
							/>
							<div className="flex flex-col text-sm">
								<span className="font-medium">Monthly Coder newsletter</span>
								<span className="text-content-secondary">
									Latest articles, workshops, events, and announcements
								</span>
							</div>
						</label>

						{/* Privacy policy notice */}
						<p className="text-xs text-content-secondary leading-relaxed">
							Subscribe for the latest product and news updates from Coder. The
							information you provide will be treated in accordance with the{" "}
							<a
								href="https://coder.com/legal/privacy-policy"
								target="_blank"
								rel="noreferrer"
								className="text-content-link hover:underline"
							>
								Coder Privacy Policy
							</a>
							.
						</p>
					</div>

					{/* Error alert */}
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

					<div className="flex justify-end">
						<Button disabled={isLoading} type="submit" data-testid="create">
							<Spinner loading={isLoading} />
							Continue
						</Button>
					</div>
				</form>

				<div className="text-xs text-content-secondary pt-6">
					&copy; {new Date().getFullYear()} Coder Technologies, Inc.
				</div>
			</div>
		</div>
	);
};
