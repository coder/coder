import * as Yup from "yup";

// Keep in sync with cli/login.go. The values are forwarded to the Coder licensor,
// so changing them requires coordinating with the licensor service.
export const numberOfDevelopersOptions = [
	"1 - 50",
	"51 - 100",
	"101 - 200",
	"201 - 500",
	"501 - 1000",
	"1001 - 2500",
	"2500+",
];

export const DATABASE_DOCS_LINK =
	"/admin/infrastructure/architecture#postgresql-recommended";

export const CODER_PRIVACY_POLICY_LINK =
	"https://coder.com/legal/privacy-policy";

export const CONTACT_SALES_LINK = "https://coder.com/contact/sales";

export const TRIAL_OFFER_TITLE = "Start a 30-day trial of Coder Premium";

export const TRIAL_OFFER_DESCRIPTION =
	"Control what agents can access, who can use which templates, and how your infrastructure scales. No credit card required.";

// REMARK: Keep these consts in sync with codersdk.CreateTrialLicenseRequest.
export const MAX_EMAIL_LENGTH = 254;
export const MAX_NAME_LENGTH = 60;
const MIN_JOB_TITLE_LENGTH = 2;
export const MAX_JOB_TITLE_LENGTH = 100;
const MIN_COMPANY_NAME_LENGTH = 2;
export const MAX_COMPANY_NAME_LENGTH = 100;
const PHONE_NUMBER_RE = /^\+?[\d\s\-.()]{7,20}$/;

export const trialInfoValidationSchema = Yup.object({
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
		.matches(PHONE_NUMBER_RE, {
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
});
