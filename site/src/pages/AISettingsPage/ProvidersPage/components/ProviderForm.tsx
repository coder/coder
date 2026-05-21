import { useFormik } from "formik";
import { TriangleAlertIcon } from "lucide-react";
import { type FC, useEffect, useId, useRef } from "react";
import { Link } from "react-router";
import * as Yup from "yup";
import type { AIProviderType } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Form, FormFields } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";
import { getFormHelpers } from "#/utils/formUtils";
import { CredentialField } from "./CredentialField";

export type ProviderFormValues = {
	type: AIProviderType | "";
	name: string;
	displayName: string;
	baseUrl: string;
	model: string;
	smallFastModel: string;
	accessKey: string;
	accessKeySecret: string;
	apiKey: string;
	enabled: boolean;
};

const HTTP_SCHEME_REGEX = /^https?:\/\//i;
const BEDROCK_CANONICAL_URL_REGEX =
	/^https:\/\/bedrock-runtime\.([a-z0-9-]+)\.amazonaws\.com\/?$/i;
const PROVIDER_NAME_REGEX = /^[a-z0-9]+(-[a-z0-9]+)*$/;

export const SAVED_CREDENTIAL_MASK = "********";

export const parseBedrockRegionFromBaseUrl = (
	baseUrl: string,
): string | undefined => {
	const match = BEDROCK_CANONICAL_URL_REGEX.exec(baseUrl.trim());
	return match?.[1]?.toLowerCase();
};

const makeNameSchema = (editing: boolean) =>
	editing
		? Yup.string()
		: Yup.string()
				.matches(
					PROVIDER_NAME_REGEX,
					"Name must be lowercase, hyphen-separated (e.g. 'my-anthropic').",
				)
				.required("Name is required");

const makeDisplayNameSchema = (editing: boolean) =>
	editing ? Yup.string().required("Display name is required") : Yup.string();

const defaultInitialValues: ProviderFormValues = {
	type: "anthropic",
	name: "",
	displayName: "",
	baseUrl: "",
	model: "",
	smallFastModel: "",
	accessKey: "",
	accessKeySecret: "",
	apiKey: "",
	enabled: true,
};

const providerDefaults: Record<AIProviderType, Partial<ProviderFormValues>> = {
	openai: { name: "openai", baseUrl: "https://api.openai.com/v1/" },
	anthropic: { name: "anthropic", baseUrl: "https://api.anthropic.com" },
	bedrock: {
		name: "bedrock",
		baseUrl: "https://bedrock-runtime.us-east-2.amazonaws.com",
	},
	azure: {
		name: "azure",
		baseUrl: "https://YOUR-RESOURCE.openai.azure.com/openai/v1",
	},
	google: {
		name: "google",
		baseUrl: "https://generativelanguage.googleapis.com/v1beta/openai/",
	},
	"openai-compat": { name: "openai-compat", baseUrl: "" },
	openrouter: { name: "openrouter", baseUrl: "https://openrouter.ai/api/v1" },
	vercel: { name: "vercel", baseUrl: "https://ai-gateway.vercel.sh/v1" },
};

const makeOpenAiAnthropicSchema = (editing: boolean) =>
	Yup.object({
		type: Yup.string()
			.oneOf([
				"openai",
				"anthropic",
				"azure",
				"google",
				"openai-compat",
				"openrouter",
				"vercel",
			] as const)
			.required(),
		name: makeNameSchema(editing),
		displayName: makeDisplayNameSchema(editing),
		baseUrl: Yup.string()
			.url("Endpoint must be a valid URL")
			.matches(HTTP_SCHEME_REGEX, "Endpoint must use http or https.")
			.required("Endpoint is required"),
		apiKey: editing
			? Yup.string()
			: Yup.string().required("API key is required"),
		enabled: Yup.boolean(),
	});

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
			.matches(
				BEDROCK_CANONICAL_URL_REGEX,
				"Endpoint must be a standard AWS Bedrock URL.",
			)
			.required("Endpoint is required"),
		apiKey: Yup.string(),
		model: Yup.string(),
		smallFastModel: Yup.string(),
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
	Yup.lazy((value: { type?: AIProviderType } | undefined) => {
		switch (value?.type) {
			case "openai":
			case "anthropic":
			case "azure":
			case "google":
			case "openai-compat":
			case "openrouter":
			case "vercel":
				return makeOpenAiAnthropicSchema(editing);
			case "bedrock":
				return makeBedrockSchema(editing);
			default:
				return Yup.object({
					type: Yup.string()
						.oneOf([
							"openai",
							"anthropic",
							"bedrock",
							"azure",
							"google",
							"openai-compat",
							"openrouter",
							"vercel",
						])
						.required(),
				});
		}
	});

type ProviderFormProps = {
	editing?: boolean;
	/** When editing Bedrock and the API already has keys, show masked placeholders until cleared. */
	bedrockSavedAccessCredentials?: boolean;
	/** When editing openai/anthropic and a key is on file, show a masked placeholder until cleared. */
	openAiAnthropicSavedApiKey?: boolean;
	/** Masked rendering of the saved openai/anthropic key (e.g. `sk-***...ABCD`). Falls back to a generic mask when omitted. */
	openAiAnthropicMaskedApiKey?: string;
	initialValues?: Partial<ProviderFormValues>;
	onSubmit?: (values: ProviderFormValues) => void;
	isLoading?: boolean;
	submitError?: unknown;
};

const namePlaceholder = (provider: string) =>
	providerDefaults[provider as keyof typeof providerDefaults]?.name;

const apiKeyPlaceholder = (provider: string) => {
	switch (provider) {
		case "openai":
			return "sk-proj-...";
		case "anthropic":
			return "sk-ant-...";
	}
};

const baseUrlPlaceholder = (provider: string) =>
	providerDefaults[provider as keyof typeof providerDefaults]?.baseUrl;

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

	const resolvedType = initialValues?.type ?? defaultInitialValues.type;
	const typeDefaults =
		providerDefaults[resolvedType as keyof typeof providerDefaults];

	const form = useFormik<ProviderFormValues>({
		initialValues: {
			...defaultInitialValues,
			// Layer order: base defaults < type prefills < parent's initialValues.
			// Edit overrides prefills with server values; create gets them as-is.
			...(typeDefaults ?? {}),
			...initialValues,
			// Seed Bedrock credentials with the mask when on file; focus clears it,
			// and a re-submitted "" tells the API mapping to keep the value.
			accessKey: bedrockSavedAccessCredentials ? SAVED_CREDENTIAL_MASK : "",
			accessKeySecret: bedrockSavedAccessCredentials
				? SAVED_CREDENTIAL_MASK
				: "",
			// Same pattern for openai/anthropic. Prefer the API-supplied masked
			// rendering so the user sees the key's identifying suffix.
			apiKey: openAiAnthropicSavedApiKey
				? (openAiAnthropicMaskedApiKey ?? SAVED_CREDENTIAL_MASK)
				: "",
		},
		validationSchema: getProviderFormSchema(editing),
		onSubmit: onSubmit ?? (() => {}),
	});
	const getFieldHelpers = getFormHelpers(form, submitError);

	const typeSelectValue = form.values.type;

	// Clears the field once if it's still showing the seeded mask;
	// subsequent focuses are no-ops.
	const handleCredentialFocus = (
		field: "apiKey" | "accessKey" | "accessKeySecret",
	) => {
		const initial = form.initialValues[field];
		if (form.values[field] === initial && initial !== "") {
			void form.setFieldValue(field, "");
		}
	};

	// When the parent's mutation finishes without an error, treat the just-
	// submitted values as the new baseline so the unsaved-changes prompt does
	// not fire on subsequent navigations.
	const previousIsLoading = useRef(isLoading);
	useEffect(() => {
		if (previousIsLoading.current && !isLoading && submitError === undefined) {
			form.resetForm({ values: form.values });
		}
		previousIsLoading.current = isLoading;
	}, [isLoading, submitError, form]);

	const unsavedChanges = useUnsavedChangesPrompt(
		form.dirty && !form.isSubmitting,
	);

	return (
		<Form onSubmit={form.handleSubmit}>
			<FormFields>
				{Boolean(submitError) && <ErrorAlert error={submitError} />}
				{typeSelectValue !== "" && typeSelectValue !== "bedrock" && (
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
								</>
							}
							className="w-full"
							placeholder={baseUrlPlaceholder(form.values.type)}
						/>
						<div className="grid grid-cols-2 items-start gap-4">
							<FormField
								field={getFieldHelpers("model")}
								label="Model"
								className="w-full"
								placeholder="anthropic.claude-3-5-sonnet-20241022-v2:0"
							/>
							<FormField
								field={{
									...getFieldHelpers("smallFastModel"),
									helperText:
										"These models are optimized for tasks like code autocomplete and other small, quick operations.",
								}}
								label="Small fast model"
								className="w-full"
								placeholder="anthropic.claude-3-haiku-20240307-v1:0"
							/>
							<CredentialField
								required
								label="Access key"
								helpers={getFieldHelpers("accessKey")}
								onFocus={() => handleCredentialFocus("accessKey")}
							/>
							<CredentialField
								required
								label="Access key secret"
								helpers={getFieldHelpers("accessKeySecret")}
								onFocus={() => handleCredentialFocus("accessKeySecret")}
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
			<ConfirmDialog
				type="info"
				hideCancel={false}
				open={unsavedChanges.isOpen}
				onClose={unsavedChanges.onCancel}
				onConfirm={unsavedChanges.onConfirm}
				title="Unsaved changes"
				confirmText="Confirm"
				description={
					<div className="flex items-start gap-3">
						<TriangleAlertIcon className="size-icon-sm mt-1 shrink-0" />
						<p className="m-0">
							Your updates haven't been saved. Leave anyway?
						</p>
					</div>
				}
			/>
		</Form>
	);
};
