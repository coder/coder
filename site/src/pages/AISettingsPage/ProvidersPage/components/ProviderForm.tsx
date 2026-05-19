import { useFormik } from "formik";
import { TrashIcon } from "lucide-react";
import { type FC, type ReactNode, useEffect, useId, useState } from "react";
import { Link } from "react-router";
import * as Yup from "yup";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Form, FormFields } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
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
import { Switch } from "#/components/Switch/Switch";
import { ProviderIcon } from "#/pages/AISettingsPage/ProvidersPage/components/ProviderIcon";
import { cn } from "#/utils/cn";
import { type FormHelpers, getFormHelpers } from "#/utils/formUtils";

export type ProviderFormValues = {
	type: "" | "openai" | "anthropic" | "bedrock";
	name: string;
	baseUrl: string;
	model: string;
	smallFastModel: string;
	accessKey: string;
	accessKeySecret: string;
	apiKey: string;
	enabled: boolean;
};

// Public AWS partition Bedrock Runtime API base URL, for example
// https://bedrock-runtime.us-east-2.amazonaws.com
const bedrockRuntimeBaseUrlRegex =
	/^https:\/\/bedrock-runtime\.[a-z0-9-]+\.amazonaws\.com\/?$/i;

// Provider names must match the kebab-case pattern enforced by the API.
const providerNameRegex = /^[a-z0-9]+(-[a-z0-9]+)*$/;
const providerNameErrorMessage =
	"Name must be lowercase, hyphen-separated (e.g. 'my-anthropic').";

// On create the user-facing name field becomes the immutable kebab-case slug
// (server enforces the same regex), so we validate the pattern up front. On
// edit the same field stores the free-form `display_name`, so we only require
// it to be non-empty.
const makeNameSchema = (editing: boolean) =>
	editing
		? Yup.string().required("Display name is required")
		: Yup.string()
				.matches(providerNameRegex, providerNameErrorMessage)
				.required("Name is required");

/**
 * Stable mask shown in credential inputs when a value already exists on the
 * server. The companion trash button next to each input clears the field so
 * the user can type a replacement; an untouched mask sanitizes to empty on
 * the wire, which the API mapping treats as "keep the existing value".
 */
export const SAVED_CREDENTIAL_MASK = "********";

const defaultInitialValues: ProviderFormValues = {
	type: "anthropic",
	name: "",
	baseUrl: "",
	model: "",
	smallFastModel: "",
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
		baseUrl: Yup.string().url("Custom endpoint must be a valid URL"),
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
		baseUrl: Yup.string()
			.url("Base URL must be a valid URL")
			.matches(
				bedrockRuntimeBaseUrlRegex,
				"Base URL must be a valid Bedrock Runtime API base URL",
			)
			.required("Base URL is required"),
		apiKey: Yup.string(),
		model: Yup.string().required("Model is required"),
		smallFastModel: Yup.string().required("Small fast model is required"),
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
	onClear: () => void;
	inputType?: "text" | "password";
	autoComplete?: string;
	placeholder?: string;
	description?: ReactNode;
	required?: boolean;
	/**
	 * Disables the input while keeping the trash button clickable. Used for
	 * the seeded-mask state: until the user presses trash, the credential
	 * input is locked at `********` so they can't accidentally edit it.
	 */
	disabled?: boolean;
	trashLabel: string;
};

/**
 * Single credential input + per-field destructive trash button. The trash
 * button stays visible at all times so the user can clear whatever they just
 * typed (or the seeded `SAVED_CREDENTIAL_MASK` when a credential is already
 * on file). Pass `inputType="password"` to render the value as dots.
 *
 * `CredentialField` mirrors `FormField`'s stacking order (label, description,
 * input, helper text) and slots a destructive trash button next to the input
 * so the user can clear a saved credential before typing a replacement.
 */
const CredentialField: FC<CredentialFieldProps> = ({
	label,
	helpers,
	onClear,
	inputType,
	autoComplete,
	placeholder,
	description,
	required = false,
	disabled = false,
	trashLabel,
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
			type={inputType}
			autoComplete={autoComplete}
			placeholder={placeholder}
			disabled={disabled}
			aria-invalid={helpers.error}
			aria-describedby={describedBy || undefined}
			className={cn("w-full", helpers.error && "border-border-destructive")}
		/>
	);

	// Only show the trash button while the input is locked at the seeded
	// credential mask. Once the user clears the field (or has been typing a
	// fresh credential since mount), the trash is hidden so the user doesn't
	// see it floating next to a half-typed key.
	const trashNode = disabled ? (
		<Button
			type="button"
			variant="destructive"
			size="icon-lg"
			onClick={onClear}
			aria-label={trashLabel}
		>
			<TrashIcon aria-hidden="true" />
			<span className="sr-only">{trashLabel}</span>
		</Button>
	) : null;

	return (
		<div className="flex flex-col gap-2">
			{labelNode}
			{descriptionNode}
			<div className="flex items-start gap-2">
				<div className="flex min-w-0 flex-1 flex-col gap-2">
					{inputNode}
					{helperNode}
				</div>
				{trashNode}
			</div>
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
			return "https://api.openai.com";
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
	const typeSelectId = useId();
	const enabledSwitchId = useId();

	// While editing, each credential input is locked at its seeded mask until
	// the user presses the trash button next to it; pressing trash flips the
	// mask boolean off and empties the form value so the user can type a
	// replacement. We track one boolean per field rather than one shared flag
	// so a user can clear (and re-enter) just one half of a paired credential.
	// The Bedrock access key (not the secret or the API key) also swaps from
	// password dots back to plain text once cleared so the user can see what
	// they're typing.
	const [bedrockAccessKeyMasked, setBedrockAccessKeyMasked] = useState(
		() => bedrockSavedAccessCredentials,
	);
	const [bedrockAccessKeySecretMasked, setBedrockAccessKeySecretMasked] =
		useState(() => bedrockSavedAccessCredentials);
	const [openAiAnthropicApiKeyMasked, setOpenAiAnthropicApiKeyMasked] =
		useState(() => openAiAnthropicSavedApiKey);

	useEffect(() => {
		setBedrockAccessKeyMasked(bedrockSavedAccessCredentials);
		setBedrockAccessKeySecretMasked(bedrockSavedAccessCredentials);
	}, [bedrockSavedAccessCredentials]);

	useEffect(() => {
		setOpenAiAnthropicApiKeyMasked(openAiAnthropicSavedApiKey);
	}, [openAiAnthropicSavedApiKey]);

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
			// shown as a mask; focusing or pressing "Clear key" clears it so the
			// user can type a replacement. Prefer the API-supplied masked
			// rendering when available so the user sees the leading/trailing
			// characters that identify which key is on file.
			apiKey: openAiAnthropicSavedApiKey
				? (openAiAnthropicMaskedApiKey ?? SAVED_CREDENTIAL_MASK)
				: "",
		},
		validationSchema: getProviderFormSchema(editing),
		onSubmit: onSubmit ?? (() => {}),
	});
	const getFieldHelpers = getFormHelpers(form, submitError);
	const typeField = getFieldHelpers("type");

	const typeSelectValue = form.values.type;

	const clearBedrockAccessKey = () => {
		void form.setFieldValue("accessKey", "");
		setBedrockAccessKeyMasked(false);
	};

	const clearBedrockAccessKeySecret = () => {
		void form.setFieldValue("accessKeySecret", "");
		setBedrockAccessKeySecretMasked(false);
	};

	const clearOpenAiAnthropicApiKey = () => {
		void form.setFieldValue("apiKey", "");
		setOpenAiAnthropicApiKeyMasked(false);
	};

	return (
		<Form onSubmit={form.handleSubmit}>
			<FormFields>
				{Boolean(submitError) && <ErrorAlert error={submitError} />}
				{!editing && (
					<div className="flex flex-col gap-2">
						<Label htmlFor={typeSelectId}>Type</Label>
						<div className="text-xs text-content-secondary">
							Select the type of provider you want to connect.
						</div>
						<Select
							value={typeSelectValue}
							onValueChange={(value) => {
								void form.setFieldValue("type", value);
							}}
						>
							<SelectTrigger
								id={typeSelectId}
								className={cn(
									"w-full",
									typeField.error && "border-border-destructive",
								)}
								aria-invalid={typeField.error}
								aria-describedby={
									typeField.error ? `${typeSelectId}-error` : undefined
								}
							>
								<SelectValue placeholder="Select type" />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="anthropic">
									<span className="flex items-center gap-2">
										<ProviderIcon provider="anthropic" />
										Anthropic
									</span>
								</SelectItem>
								<SelectItem value="openai">
									<span className="flex items-center gap-2">
										<ProviderIcon provider="openai" />
										OpenAI
									</span>
								</SelectItem>
								<SelectItem value="bedrock">
									<span className="flex items-center gap-2">
										<ProviderIcon provider="bedrock" />
										Bedrock
									</span>
								</SelectItem>
							</SelectContent>
						</Select>
						{typeField.error ? (
							<span
								id={`${typeSelectId}-error`}
								className="text-xs text-content-destructive"
							>
								{typeField.helperText}
							</span>
						) : null}
					</div>
				)}

				{(typeSelectValue === "openai" || typeSelectValue === "anthropic") && (
					<>
						<FormField
							required
							field={getFieldHelpers("name")}
							label={editing ? "Display name" : "Name"}
							description={
								editing
									? "A friendly name shown for this provider in the UI. The original identifier cannot be changed."
									: "The name of the provider. This is used to identify the provider in the UI."
							}
							className="w-full"
							placeholder={namePlaceholder(form.values.type)}
						/>
						{/* API keys live on a sub-resource server-side; the parent
						    page chains POST /keys (and revokes the previous key when
						    rotating) after the provider PATCH succeeds. We treat an
						    untouched mask as "keep the existing key". */}
						<CredentialField
							required
							label="API key"
							helpers={getFieldHelpers("apiKey")}
							onClear={clearOpenAiAnthropicApiKey}
							// While the field is showing a saved masked value
							// (e.g. `sk-ant-***\u2026***ABCD`), render as plain text so
							// the user can see the identifying suffix the API returned.
							// Once the user clears the input to type a new key, switch
							// back to password so the plaintext isn't shoulder-surfable.
							inputType={openAiAnthropicApiKeyMasked ? "text" : "password"}
							autoComplete="new-password"
							description="Secret key used to authenticate requests to this provider."
							placeholder={apiKeyPlaceholder(form.values.type)}
							disabled={openAiAnthropicApiKeyMasked}
							trashLabel="Remove saved API key"
						/>
						<FormField
							field={getFieldHelpers("baseUrl")}
							label="Custom endpoint"
							description="Custom endpoint for this provider. Leave empty to use the default."
							className="w-full"
							placeholder={baseUrlPlaceholder(form.values.type)}
						/>
					</>
				)}

				{typeSelectValue === "bedrock" && (
					<>
						<FormField
							required
							field={getFieldHelpers("name")}
							label={editing ? "Display name" : "Name"}
							description={
								editing
									? "A friendly name shown for this provider in the UI. The original identifier cannot be changed."
									: "The name of the provider. This is used to identify the provider in the UI."
							}
							className="w-full"
							placeholder={namePlaceholder(form.values.type)}
						/>
						<FormField
							required
							field={getFieldHelpers("baseUrl")}
							label="Base URL"
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
						<div className="grid grid-cols-2 items-start gap-4">
							<FormField
								required
								field={getFieldHelpers("model")}
								label="Model"
								description="The primary Bedrock model ID used for chat and completion requests."
								className="w-full"
								placeholder="anthropic.claude-3-5-sonnet-20241022-v2:0"
							/>
							<FormField
								required
								field={getFieldHelpers("smallFastModel")}
								label="Small fast model"
								description="A cheaper, faster model used for lightweight tasks such as summaries, titles, and routing."
								className="w-full"
								placeholder="anthropic.claude-3-haiku-20240307-v1:0"
							/>
						</div>
						<CredentialField
							required
							label="Access key"
							helpers={getFieldHelpers("accessKey")}
							onClear={clearBedrockAccessKey}
							// Hide the access key value when masked so it renders
							// uniformly with the secret; revert to plain text once
							// cleared so the typed key is visible.
							inputType={bedrockAccessKeyMasked ? "password" : "text"}
							description="Your AWS Access Key ID used to authenticate requests to Bedrock."
							disabled={bedrockAccessKeyMasked}
							trashLabel="Remove saved access key"
						/>
						<CredentialField
							required
							label="Access key secret"
							helpers={getFieldHelpers("accessKeySecret")}
							onClear={clearBedrockAccessKeySecret}
							inputType="password"
							autoComplete="new-password"
							description="Your AWS Secret Access Key associated with the access key ID. Stored securely and used for request signing."
							disabled={bedrockAccessKeySecretMasked}
							trashLabel="Remove saved access key secret"
						/>
					</>
				)}

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
