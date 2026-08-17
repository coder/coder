import { useFormik } from "formik";
import { type FC, useId } from "react";
import * as Yup from "yup";
import { countries } from "#/api/countriesGenerated";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { FormField } from "#/components/FormField/FormField";
import { SelectItem } from "#/components/Select/Select";
import { SelectField } from "#/components/SelectField/SelectField";
import { Spinner } from "#/components/Spinner/Spinner";
import { numberOfDevelopersOptions } from "#/modules/licenses/trialLicense";
import { docs } from "#/utils/docs";
import { getFormHelpers, onChangeTrimmed } from "#/utils/formUtils";

const DATABASE_DOCS_LINK = docs(
	"/admin/infrastructure/architecture#postgresql-recommended",
);

type TrialFormValues = TypesGen.CreateTrialLicenseRequest & {
	// client-side gate only
	acknowledged: boolean;
};

// REMARK: Keep these consts in sync with codersdk.CreateTrialLicenseRequest.
const MAX_EMAIL_LENGTH = 254;
const MAX_NAME_LENGTH = 60;
const MIN_JOB_TITLE_LENGTH = 2;
const MAX_JOB_TITLE_LENGTH = 100;
const MIN_COMPANY_NAME_LENGTH = 2;
const MAX_COMPANY_NAME_LENGTH = 100;
const E164_PHONE_NUMBER_RE = /^\+[1-9]\d{1,14}$/;

const validationSchema = Yup.object({
	email: Yup.string()
		.trim()
		.email("Please enter a valid email address.")
		.max(
			MAX_EMAIL_LENGTH,
			`Email address should be no longer than ${MAX_EMAIL_LENGTH} characters.`,
		)
		.required("Please enter an email address."),
	first_name: Yup.string()
		.max(
			MAX_NAME_LENGTH,
			`First name should be no longer than ${MAX_NAME_LENGTH} characters.`,
		)
		.required("Please enter your first name."),
	last_name: Yup.string()
		.max(
			MAX_NAME_LENGTH,
			`Last name should be no longer than ${MAX_NAME_LENGTH} characters.`,
		)
		.required("Please enter your last name."),
	phone_number: Yup.string()
		.matches(E164_PHONE_NUMBER_RE, {
			message:
				"Phone number should be in international format (e.g. +14155552671).",
			excludeEmptyString: true,
		})
		.required("Please enter your phone number."),
	job_title: Yup.string()
		.min(
			MIN_JOB_TITLE_LENGTH,
			`Job title should be at least ${MIN_JOB_TITLE_LENGTH} characters.`,
		)
		.max(
			MAX_JOB_TITLE_LENGTH,
			`Job title should be no longer than ${MAX_JOB_TITLE_LENGTH} characters.`,
		)
		.required("Please enter your job title."),
	company_name: Yup.string()
		.min(
			MIN_COMPANY_NAME_LENGTH,
			`Company name should be at least ${MIN_COMPANY_NAME_LENGTH} characters.`,
		)
		.max(
			MAX_COMPANY_NAME_LENGTH,
			`Company name should be no longer than ${MAX_COMPANY_NAME_LENGTH} characters.`,
		)
		.required("Please enter your company name."),
	country: Yup.string().required("Please select your country."),
	developers: Yup.string().required(
		"Please select the number of developers in your company.",
	),
	acknowledged: Yup.bool().oneOf(
		[true],
		"Please acknowledge the database requirements.",
	),
});

const initialValues: TrialFormValues = {
	email: "",
	first_name: "",
	last_name: "",
	phone_number: "",
	job_title: "",
	company_name: "",
	country: "",
	developers: "",
	acknowledged: false,
};

interface TrialRequestFormProps {
	onSubmit: (request: TypesGen.CreateTrialLicenseRequest) => void;
	isSubmitting: boolean;
	error?: unknown;
}

export const TrialRequestForm: FC<TrialRequestFormProps> = ({
	onSubmit,
	isSubmitting,
	error,
}) => {
	const acknowledgementId = useId();
	const form = useFormik<TrialFormValues>({
		initialValues,
		validationSchema,
		onSubmit: (values) => {
			// Built field by field so the acknowledgement cannot reach the licensor.
			onSubmit({
				email: values.email,
				first_name: values.first_name,
				last_name: values.last_name,
				phone_number: values.phone_number,
				job_title: values.job_title,
				company_name: values.company_name,
				country: values.country,
				developers: values.developers,
			});
		},
	});
	const getFieldHelpers = getFormHelpers<TrialFormValues>(form, error);

	return (
		<form onSubmit={form.handleSubmit} className="flex flex-col gap-6">
			<div className="flex flex-col gap-2">
				<h1 className="m-0 font-semibold text-2xl text-content-primary">
					Start your Premium trial
				</h1>
				<p className="m-0 text-sm text-content-secondary">
					Tell us how to reach you and we will activate a 30-day Premium trial
					on this deployment.
				</p>
			</div>

			<div className="flex flex-col gap-4">
				<FormField
					label="Email"
					type="email"
					field={getFieldHelpers("email", { maxLength: MAX_EMAIL_LENGTH })}
					onChange={onChangeTrimmed(form)}
					disabled={isSubmitting}
				/>

				<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
					<FormField
						label="First name"
						field={getFieldHelpers("first_name", {
							maxLength: MAX_NAME_LENGTH,
						})}
						disabled={isSubmitting}
					/>
					<FormField
						label="Last name"
						field={getFieldHelpers("last_name", { maxLength: MAX_NAME_LENGTH })}
						disabled={isSubmitting}
					/>
				</div>

				<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
					<FormField
						label="Company"
						field={getFieldHelpers("company_name", {
							maxLength: MAX_COMPANY_NAME_LENGTH,
						})}
						disabled={isSubmitting}
					/>
					<FormField
						label="Job title"
						field={getFieldHelpers("job_title", {
							maxLength: MAX_JOB_TITLE_LENGTH,
						})}
						disabled={isSubmitting}
					/>
				</div>

				<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
					<FormField
						label="Phone number"
						field={getFieldHelpers("phone_number")}
						disabled={isSubmitting}
					/>
					<SelectField
						label="Number of developers"
						{...getFieldHelpers("developers")}
						onValueChange={(value: string) =>
							form.setFieldValue("developers", value)
						}
						placeholder="Select..."
						disabled={isSubmitting}
					>
						{numberOfDevelopersOptions.map((opt) => (
							<SelectItem key={opt} value={opt}>
								{opt}
							</SelectItem>
						))}
					</SelectField>
				</div>

				<SelectField
					label="Country"
					{...getFieldHelpers("country")}
					onValueChange={(value: string) =>
						form.setFieldValue("country", value)
					}
					placeholder="Select..."
					disabled={isSubmitting}
				>
					{countries.map((c) => (
						<SelectItem key={c.name} value={c.name}>
							{c.flag} {c.name}
						</SelectItem>
					))}
				</SelectField>
			</div>

			<div className="flex flex-col gap-2">
				<label
					htmlFor={acknowledgementId}
					className="flex cursor-pointer gap-2 items-start text-sm"
				>
					<Checkbox
						id={acknowledgementId}
						checked={form.values.acknowledged}
						onCheckedChange={(checked) =>
							form.setFieldValue("acknowledged", checked === true)
						}
						disabled={isSubmitting}
					/>
					<span className="text-content-secondary">
						I understand that Premium features increase database load, and that
						Coder recommends an external PostgreSQL database for production
						deployments.
					</span>
				</label>
				<a
					href={DATABASE_DOCS_LINK}
					target="_blank"
					rel="noreferrer"
					className="ml-6 text-xs text-content-link hover:underline w-fit"
				>
					Learn more
				</a>
			</div>

			<div className="flex flex-col gap-2 items-start">
				<Button
					type="submit"
					size="lg"
					disabled={!form.values.acknowledged || isSubmitting}
				>
					<Spinner loading={isSubmitting} />
					Start trial
				</Button>
				{!form.values.acknowledged && (
					<p className="m-0 text-xs text-content-secondary">
						Acknowledge the database requirements to start your trial.
					</p>
				)}
			</div>
		</form>
	);
};
