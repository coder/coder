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
import { PrivacyPolicyNotice } from "#/modules/licenses/PrivacyPolicyNotice";
import {
	DATABASE_DOCS_LINK,
	MAX_COMPANY_NAME_LENGTH,
	MAX_EMAIL_LENGTH,
	MAX_JOB_TITLE_LENGTH,
	MAX_NAME_LENGTH,
	numberOfDevelopersOptions,
	trialInfoValidationSchema,
} from "#/modules/licenses/trialLicense";
import { docs } from "#/utils/docs";
import { getFormHelpers, onChangeTrimmed } from "#/utils/formUtils";

type TrialFormValues = TypesGen.CreateTrialLicenseRequest & {
	// client-side gate only
	acknowledged: boolean;
};

const validationSchema = trialInfoValidationSchema.shape({
	email: Yup.string()
		.trim()
		.email("Please enter a valid email address.")
		.max(
			MAX_EMAIL_LENGTH,
			`Email address should be no longer than ${MAX_EMAIL_LENGTH} characters.`,
		)
		.required("Please enter an email address."),
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
		<form
			onSubmit={form.handleSubmit}
			className="flex flex-col gap-6"
			noValidate
		>
			<div className="flex flex-col gap-4">
				<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
					<FormField
						label="First name"
						placeholder="Jane"
						field={getFieldHelpers("first_name", {
							maxLength: MAX_NAME_LENGTH,
						})}
						disabled={isSubmitting}
					/>
					<FormField
						label="Last name"
						placeholder="Doe"
						field={getFieldHelpers("last_name", { maxLength: MAX_NAME_LENGTH })}
						disabled={isSubmitting}
					/>
				</div>

				<FormField
					label="Business email"
					type="email"
					placeholder="you@company.com"
					field={getFieldHelpers("email", { maxLength: MAX_EMAIL_LENGTH })}
					onChange={onChangeTrimmed(form)}
					disabled={isSubmitting}
				/>

				<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
					<FormField
						label="Company"
						placeholder="Acme Inc."
						field={getFieldHelpers("company_name", {
							maxLength: MAX_COMPANY_NAME_LENGTH,
						})}
						disabled={isSubmitting}
					/>
					<FormField
						label="Job title"
						placeholder="Platform Engineer"
						field={getFieldHelpers("job_title", {
							maxLength: MAX_JOB_TITLE_LENGTH,
						})}
						disabled={isSubmitting}
					/>
				</div>

				<div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
					<FormField
						type="tel"
						label="Phone number"
						placeholder="+1 415 5552671"
						field={getFieldHelpers("phone_number")}
						disabled={isSubmitting}
					/>
					<SelectField
						label="Number of developers"
						field={getFieldHelpers("developers")}
						onValueChange={(value) => form.setFieldValue("developers", value)}
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
					field={getFieldHelpers("country")}
					onValueChange={(value) => form.setFieldValue("country", value)}
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
			<div className="flex gap-2 items-start text-sm text-content-primary">
				<Checkbox
					id={acknowledgementId}
					checked={form.values.acknowledged}
					onCheckedChange={(checked) =>
						form.setFieldValue("acknowledged", checked === true)
					}
					disabled={isSubmitting}
				/>
				<div>
					<label htmlFor={acknowledgementId} className="cursor-pointer">
						I understand that Coder trial features increase database load, and
						that Coder recommends an external PostgreSQL database for production
						deployments.
					</label>{" "}
					<a
						href={docs(DATABASE_DOCS_LINK)}
						target="_blank"
						rel="noreferrer"
						className="text-content-link hover:underline"
					>
						Learn more
					</a>
				</div>
			</div>

			<div className="flex flex-col gap-2">
				<Button
					type="submit"
					size="lg"
					className="w-full"
					disabled={!form.values.acknowledged || isSubmitting}
				>
					<Spinner loading={isSubmitting} />
					Start a trial
				</Button>
				<p className="m-0 text-xs text-content-secondary leading-relaxed">
					<PrivacyPolicyNotice /> Opt-out at any time.
				</p>
			</div>
		</form>
	);
};
