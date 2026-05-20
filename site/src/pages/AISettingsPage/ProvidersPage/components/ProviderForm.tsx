import { useFormik } from "formik";
import { type FC, type ReactNode, useId } from "react";
import { Link } from "react-router";
import * as Yup from "yup";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Form, FormFields } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import { cn } from "#/utils/cn";
import { type FormHelpers, getFormHelpers } from "#/utils/formUtils";

export type ProviderFormValues = {
	type: "" | "openai" | "anthropic" | "bedrock";
	name: string;
	displayName: string;
	baseUrl: string;
	model: string;
	smallFastModel: string;
	region: string;
	accessKey: string;
	accessKeySecret: string;
	apiKey: string;
	enabled: boolean;
};

// Server requires the base_url scheme to be http or https; Yup's `.url()`
// otherwise lets through any RFC URL (ftp://..., mailto:..., etc.). Anchored
// at the start so substrings like "my-https://..." still fail.
const httpSchemeRegex = /^https?:\/\//i;
const httpSchemeErrorMessage = "Endpoint must use http or https.";

// Canonical AWS Bedrock Runtime URL, e.g.
// `https://bedrock-runtime.us-east-1.amazonaws.com` (with optional trailing
// slash). When the endpoint matches this pattern the form derives the AWS
// region from the URL instead of asking the user; non-canonical endpoints
// (proxies, sandboxes, GovCloud non-bedrock-runtime hosts) fall through and
// surface an explicit Region field.
const bedrockCanonicalUrlRegex =
	/^https:\/\/bedrock-runtime\.([a-z0-9-]+)\.amazonaws\.com\/?$/i;

/**
 * Returns the AWS region encoded in the URL when it matches the canonical AWS
 * Bedrock Runtime pattern, otherwise `undefined`. Regions are lowercased to
 * match AWS conventions; the AWS SDK accepts uppercase but normalizes
 * internally.
 */
export const parseBedrockRegionFromBaseUrl = (
	baseUrl: string,
): string | undefined => {
	const match = bedrockCanonicalUrlRegex.exec(baseUrl.trim());
	return match?.[1]?.toLowerCase();
};

// Provider names must match the kebab-case pattern enforced by the API.
const providerNameRegex = /^[a-z0-9]+(-[a-z0-9]+)*$/;
const providerNameErrorMessage =
	"Name must be lowercase, hyphen-separated (e.g. 'my-anthropic').";

// Slug is immutable server-side. We only validate it on create; on edit the
// form hides the field and the seeded value (read straight back from the API)
// is already known to satisfy the regex.
const makeNameSchema = (editing: boolean) =>
	editing
		? Yup.string()
		: Yup.string()
				.matches(providerNameRegex, providerNameErrorMessage)
				.required("Name is required");

// Display name is required on edit (we always have one to send), optional on
// create (server treats empty as "no display name", and the UI falls back to
// the slug).
const makeDisplayNameSchema = (editing: boolean) =>
	editing ? Yup.string().required("Display name is required") : Yup.string();

/**
 * Stable mask shown in credential inputs when a value already exists on the
 * server. Focusing the input clears the seeded mask so the user can type a
 * replacement; an untouched mask sanitizes to empty on the wire, which the
 * API mapping treats as "keep the existing value".
 */
export const SAVED_CREDENTIAL_MASK = "********";

const defaultInitialValues: ProviderFormValues = {
	type: "anthropic",
	name: "",
	displayName: "",
	baseUrl: "",
	model: "",
	smallFastModel: "",
	region: "",
	accessKey: "",
	accessKeySecret: "",
	apiKey: "",
	enabled: true,
};

const makeOpenAiAnthropicSchema = (editing: boolean) =>
	Yup.object({
		type: Yup.string()
			.oneOf(["openai", "anthropic"] as const)
			.required(),
		name: makeNameSchema(editing),
		displayName: makeDisplayNameSchema(editing),
		baseUrl: Yup.string()
			.url("Endpoint must be a valid URL")
			.matches(httpSchemeRegex, httpSchemeErrorMessage)
			.required("Endpoint is required"),
		apiKey: editing
			? Yup.string()
			: Yup.string().required("API key is required"),
		enabled: Yup.boolean(),
	});

// Treat the saved-credential mask as empty: a value matching the placeholder
// should never be treated as a real, user-supplied credential during
// validation.
const credentialFilled = (value: string | undefined): boolean => {
	if (!value) return false;
	const trimmed = value.trim();
	return trimmed !== "" && trimmed !== SAVED_CREDENTIAL_MASK;
};

const makeBedrockSchema = (editing: boolean) =>
	Yup.object({
		type: Yup.string()
			.oneOf(["bedrock"] as const)
			.required(),
		name: makeNameSchema(editing),
		displayName: makeDisplayNameSchema(editing),
		baseUrl: Yup.string()
			.url("Endpoint must be a valid URL")
			.matches(httpSchemeRegex, httpSchemeErrorMessage)
			.required("Endpoint is required"),
		apiKey: Yup.string(),
		model: Yup.string().required("Model is required"),
		smallFastModel: Yup.string().required("Small fast model is required"),
		// Region is implicit when the URL matches the canonical AWS Bedrock
		// pattern (we extract it on submit). It is also skipped while baseUrl
		// is blank so the user only sees one error at a time. Otherwise the
		// AWS SDK needs an explicit region for SigV4 signing.
		region: Yup.string().when("baseUrl", {
			is: (baseUrl: string | undefined) => {
				const trimmed = baseUrl?.trim() ?? "";
				return (
					trimmed === "" || parseBedrockRegionFromBaseUrl(trimmed) !== undefined
				);
			},
			then: (schema) => schema,
			otherwise: (schema) => schema.required("Region is required"),
		}),
		accessKey: (editing
			? Yup.string()
			: Yup.string().required("Access key is required")
		).test(
			"access-key-paired",
			"Enter both access key and secret to rotate credentials.",
			function (value) {
				const secret = (this.parent as { accessKeySecret?: string })
					.accessKeySecret;
				return !(credentialFilled(secret) && !credentialFilled(value));
			},
		),
		accessKeySecret: (editing
			? Yup.string()
			: Yup.string().required("Access key secret is required")
		).test(
			"access-key-secret-paired",
			"Enter both access key and secret to rotate credentials.",
			function (value) {
				const accessKey = (this.parent as { accessKey?: string }).accessKey;
				return !(credentialFilled(accessKey) && !credentialFilled(value));
			},
		),
		enabled: Yup.boolean(),
	});

const getProviderFormSchema = (editing: boolean) =>
	Yup.lazy((value: { type?: string } | undefined) => {
		switch (value?.type) {
			case "openai":
			case "anthropic":
				return makeOpenAiAnthropicSchema(editing);
			case "bedrock":
				return makeBedrockSchema(editing);
			default:
				return Yup.object({
					type: Yup.string()
						.oneOf(["openai", "anthropic", "bedrock"])
						.required(),
				});
		}
	});

type CredentialFieldProps = {
	label: string;
	helpers: FormHelpers;
	inputType?: "text" | "password";
	autoComplete?: string;
	placeholder?: string;
	description?: ReactNode;
	required?: boolean;
	/**
	 * Fires when the input first receives focus. Used to clear the seeded
	 * `SAVED_CREDENTIAL_MASK` so the user can type a replacement without
	 * having to first delete the mask by hand.
	 */
	onFocus?: () => void;
};

/**
 * Single credential input. When a credential is already on file the parent
 * seeds the value with `SAVED_CREDENTIAL_MASK` (or an API-supplied masked
 * rendering) and wires up `onFocus` so the field clears on first focus,
 * letting the user type a fresh value. Pass `inputType="password"` to render
 * the value as dots; default `text` keeps the seeded mask legible.
 *
 * `CredentialField` mirrors `FormField`'s stacking order (label, description,
 * input, helper text).
 */
const CredentialField: FC<CredentialFieldProps> = ({
	label,
	helpers,
	inputType,
	autoComplete,
	placeholder,
	description,
	required = false,
	onFocus,
}) => {
	const inputId = useId();
	const errorId = `${inputId}-error`;
	const helperId = `${inputId}-helper`;
	const descriptionId = `${inputId}-description`;
	const describedBy = [
		description ? descriptionId : null,
		helpers.error ? errorId : helpers.helperText ? helperId : null,
	]
		.filter(Boolean)
		.join(" ");

	const labelNode = (
		<Label htmlFor={inputId}>
			{label}{" "}
			{required && (
				<span className="text-xs font-bold text-content-destructive">*</span>
			)}
		</Label>
	);

	const descriptionNode = description && (
		<div id={descriptionId} className="text-xs text-content-secondary">
			{description}
		</div>
	);

	const helperNode = helpers.error ? (
		<span id={errorId} className="text-xs text-content-destructive">
			{helpers.helperText}
		</span>
	) : helpers.helperText ? (
		<span id={helperId} className="text-xs text-content-secondary">
			{helpers.helperText}
		</span>
	) : null;

	const inputNode = (
		<Input
			id={inputId}
			name={helpers.name}
			value={helpers.value}
			onChange={helpers.onChange}
			onBlur={helpers.onBlur}
			onFocus={onFocus}
			type={inputType}
			autoComplete={autoComplete}
			placeholder={placeholder}
			aria-invalid={helpers.error}
			aria-describedby={describedBy || undefined}
			className={cn("w-full", helpers.error && "border-border-destructive")}
		/>
	);

	return (
		<div className="flex flex-col gap-2">
			{labelNode}
			{descriptionNode}
			{inputNode}
			{helperNode}
		</div>
	);
};

type ProviderFormProps = {
	editing?: boolean;
	/** When editing Bedrock and the API already has keys, show masked placeholders until cleared. */
	bedrockSavedAccessCredentials?: boolean;
	/** When editing openai/anthropic and a key is on file, show a masked placeholder until cleared. */
	openAiAnthropicSavedApiKey?: boolean;
	/**
	 * Masked rendering of the saved openai/anthropic key returned by the API
	 * (e.g. `"sk-***\u2026***ABCD"`). Seeded into the input when
	 * `openAiAnthropicSavedApiKey` is true; ignored otherwise. Falls back to a
	 * generic `********` mask when omitted.
	 */
	openAiAnthropicMaskedApiKey?: string;
	initialValues?: Partial<ProviderFormValues>;
	onSubmit?: (values: ProviderFormValues) => void;
	isLoading?: boolean;
	submitError?: unknown;
};

const namePlaceholder = (provider: string) => {
	switch (provider) {
		case "openai":
			return "openai";
		case "anthropic":
			return "anthropic";
		case "bedrock":
			return "bedrock";
	}
};

const apiKeyPlaceholder = (provider: string) => {
	switch (provider) {
		case "openai":
			return "sk-proj-...";
		case "anthropic":
			return "sk-ant-...";
	}
};

const baseUrlPlaceholder = (provider: string) => {
	switch (provider) {
		case "openai":
			return "https://api.openai.com/v1/";
		case "anthropic":
			return "https://api.anthropic.com";
		case "bedrock":
			return "https://bedrock-runtime.us-east-2.amazonaws.com";
		default:
			return;
	}
};

export const ProviderForm: FC<ProviderFormProps> = ({
	editing = false,
	bedrockSavedAccessCredentials = false,
	openAiAnthropicSavedApiKey = false,
	openAiAnthropicMaskedApiKey,
	initialValues,
	onSubmit,
	isLoading = false,
	submitError,
}) => {
	const enabledSwitchId = useId();

	const form = useFormik<ProviderFormValues>({
		initialValues: {
			...defaultInitialValues,
			...initialValues,
			// When the server has saved Bedrock credentials, seed the inputs
			// with the mask so the user sees something is on file. The mask
			// is replaced (cleared) on focus, and any "" submitted back is
			// treated by the API mapping as "keep the existing value".
			accessKey: bedrockSavedAccessCredentials ? SAVED_CREDENTIAL_MASK : "",
			accessKeySecret: bedrockSavedAccessCredentials
				? SAVED_CREDENTIAL_MASK
				: "",
			// Mirror the Bedrock pattern for openai/anthropic. A key on file is
			// shown as a mask; focusing the input clears it so the user can type
			// a replacement. Prefer the API-supplied masked rendering when
			// available so the user sees the leading/trailing characters that
			// identify which key is on file.
			apiKey: openAiAnthropicSavedApiKey
				? (openAiAnthropicMaskedApiKey ?? SAVED_CREDENTIAL_MASK)
				: "",
		},
		validationSchema: getProviderFormSchema(editing),
		onSubmit: onSubmit ?? (() => {}),
	});
	const getFieldHelpers = getFormHelpers(form, submitError);

	const typeSelectValue = form.values.type;

	// When a credential input is showing the seeded mask (its current value
	// still matches the non-empty initial value), the first focus clears the
	// field so the user can type a replacement. Once the user has typed (or
	// cleared) anything, subsequent focus events become no-ops.
	const handleCredentialFocus = (
		field: "apiKey" | "accessKey" | "accessKeySecret",
	) => {
		const initial = form.initialValues[field];
		if (form.values[field] === initial && initial !== "") {
			void form.setFieldValue(field, "");
		}
	};

	return (
		<Form onSubmit={form.handleSubmit}>
			<FormFields>
				{Boolean(submitError) && <ErrorAlert error={submitError} />}
				{(typeSelectValue === "openai" || typeSelectValue === "anthropic") && (
					<>
						{!editing && (
							<FormField
								required
								field={getFieldHelpers("name")}
								label="Name"
								description="Unique identifier for this provider. Used in URLs and cannot be changed later."
								className="w-full"
								placeholder={namePlaceholder(form.values.type)}
							/>
						)}
						<FormField
							required={editing}
							field={getFieldHelpers("displayName")}
							label="Display name"
							description={
								editing
									? "A friendly name shown for this provider in the UI."
									: "A friendly name shown for this provider in the UI. Defaults to the identifier if left blank."
							}
							className="w-full"
						/>
						{/* API keys live on a sub-resource server-side; the parent
						    page chains POST /keys (and revokes the previous key when
						    rotating) after the provider PATCH succeeds. We treat an
						    untouched mask as "keep the existing key". */}
						<CredentialField
							required
							label="API key"
							helpers={getFieldHelpers("apiKey")}
							onFocus={() => handleCredentialFocus("apiKey")}
							// While the field is showing a saved masked value
							// (e.g. `sk-ant-***\u2026***ABCD`), render as plain text so
							// the user can see the identifying suffix the API returned.
							// Once the user focuses the input to type a new key, switch
							// back to password so the plaintext isn't shoulder-surfable.
							inputType={
								form.values.apiKey === form.initialValues.apiKey &&
								form.initialValues.apiKey !== ""
									? "text"
									: "password"
							}
							autoComplete="new-password"
							description="Secret key used to authenticate requests to this provider."
							placeholder={apiKeyPlaceholder(form.values.type)}
						/>
						<FormField
							required
							field={getFieldHelpers("baseUrl")}
							label="Endpoint"
							description="The base URL where the provider's API is hosted."
							className="w-full"
							placeholder={baseUrlPlaceholder(form.values.type)}
						/>
					</>
				)}

				{typeSelectValue === "bedrock" && (
					<>
						{!editing && (
							<FormField
								required
								field={getFieldHelpers("name")}
								label="Name"
								description="Unique identifier for this provider. Used in URLs and cannot be changed later."
								className="w-full"
								placeholder={namePlaceholder(form.values.type)}
							/>
						)}
						<FormField
							required={editing}
							field={getFieldHelpers("displayName")}
							label="Display name"
							description={
								editing
									? "A friendly name shown for this provider in the UI."
									: "A friendly name shown for this provider in the UI. Defaults to the identifier if left blank."
							}
							className="w-full"
						/>
						<FormField
							required
							field={getFieldHelpers("baseUrl")}
							label="Endpoint"
							description={
								<>
									In the format of{" "}
									<code>
										{"https://bedrock-runtime.{region}.amazonaws.com"}
									</code>
									.
								</>
							}
							className="w-full"
							placeholder={baseUrlPlaceholder(form.values.type)}
						/>
						{form.values.baseUrl.trim() !== "" &&
							parseBedrockRegionFromBaseUrl(form.values.baseUrl) ===
								undefined && (
								<FormField
									required
									field={getFieldHelpers("region")}
									label="Region"
									description="AWS region used to sign requests. Required when the endpoint isn't a standard AWS Bedrock URL."
									className="w-full"
									placeholder="us-east-1"
								/>
							)}
						<div className="grid grid-cols-2 items-start gap-4">
							<FormField
								required
								field={getFieldHelpers("model")}
								label="Model"
								className="w-full"
								placeholder="anthropic.claude-3-5-sonnet-20241022-v2:0"
							/>
							<FormField
								required
								field={getFieldHelpers("smallFastModel")}
								label="Small fast model"
								className="w-full"
								placeholder="anthropic.claude-3-haiku-20240307-v1:0"
							/>
							<CredentialField
								required
								label="Access key"
								helpers={getFieldHelpers("accessKey")}
								onFocus={() => handleCredentialFocus("accessKey")}
								// Access keys are identifiers (not secrets), so we keep
								// them legible at all times. The seeded mask renders as
								// text and any replacement the user types stays visible.
								inputType="text"
							/>
							<CredentialField
								required
								label="Access key secret"
								helpers={getFieldHelpers("accessKeySecret")}
								onFocus={() => handleCredentialFocus("accessKeySecret")}
								// Keep the seeded mask legible (it isn't the real secret)
								// but switch to password once the user focuses to type a
								// replacement so the plaintext isn't shoulder-surfable.
								inputType={
									form.values.accessKeySecret ===
										form.initialValues.accessKeySecret &&
									form.initialValues.accessKeySecret !== ""
										? "text"
										: "password"
								}
								autoComplete="new-password"
							/>
						</div>
					</>
				)}

				{editing && (
					<div className="flex items-center justify-between gap-4">
						<div className="flex min-w-0 flex-1 flex-col gap-2">
							<Label htmlFor={enabledSwitchId}>Enabled</Label>
							<p className="m-0 text-xs text-content-secondary">
								When disabled, this provider is not available for usage.
							</p>
						</div>
						<Switch
							id={enabledSwitchId}
							checked={form.values.enabled}
							onCheckedChange={(checked) => {
								void form.setFieldValue("enabled", checked);
							}}
							disabled={isLoading}
							aria-label="Provider enabled"
						/>
					</div>
				)}

				<div className="flex justify-end gap-4">
					<Link to="/ai/settings">
						<Button variant="outline" type="button">
							Cancel
						</Button>
					</Link>
					<Button disabled={isLoading} type="submit">
						<Spinner loading={isLoading} />
						{editing ? "Update provider" : "Add provider"}
					</Button>
				</div>
			</FormFields>
		</Form>
	);
};
