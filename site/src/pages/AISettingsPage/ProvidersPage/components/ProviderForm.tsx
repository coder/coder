import { useFormik } from "formik";
import { TriangleAlertIcon } from "lucide-react";
import { type FC, useEffect, useRef } from "react";
import { Link } from "react-router";
import * as Yup from "yup";
import type { AIProviderType } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Form, FormFields } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { Link as DocsLink } from "#/components/Link/Link";
import { Spinner } from "#/components/Spinner/Spinner";
import { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";
import { docs } from "#/utils/docs";
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
	roleArn: string;
	region: string;
	workspaceId: string;
	externalId: string;
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

// Display name is always optional. The form copy says blank falls back
// to the provider name, and the update API supports clearing the value.
const makeDisplayNameSchema = (_editing: boolean) => Yup.string();

const defaultInitialValues: ProviderFormValues = {
	type: "anthropic",
	name: "",
	displayName: "",
	baseUrl: "",
	model: "",
	smallFastModel: "",
	accessKey: "",
	accessKeySecret: "",
	roleArn: "",
	region: "",
	workspaceId: "",
	externalId: "",
	apiKey: "",
	enabled: true,
};

// Bedrock model defaults mirror codersdk/deployment.go's
// aiGatewayBedrockModel and aiGatewayBedrockSmallFastModel defaults
// so the create form lands on the same models the env-seeded path
// uses. Update both sides together when AWS publishes new model IDs.
const BEDROCK_DEFAULT_MODEL =
	"global.anthropic.claude-sonnet-4-5-20250929-v1:0";
const BEDROCK_DEFAULT_SMALL_FAST_MODEL =
	"global.anthropic.claude-haiku-4-5-20251001-v1:0";
const BEDROCK_MODEL_CARDS_URL =
	"https://docs.aws.amazon.com/bedrock/latest/userguide/model-cards.html";

// Default Claude Platform for AWS endpoint. Region defaults to us-east-1;
// operators adjust the region in the URL and the dedicated Region field.
const CLAUDE_PLATFORM_DEFAULT_BASE_URL =
	"https://aws-external-anthropic.us-east-1.api.aws";

const providerDefaults: Partial<
	Record<AIProviderType, Partial<ProviderFormValues>>
> = {
	openai: { name: "openai", baseUrl: "https://api.openai.com/v1/" },
	anthropic: { name: "anthropic", baseUrl: "https://api.anthropic.com" },
	bedrock: {
		name: "bedrock",
		baseUrl: "https://bedrock-runtime.us-east-2.amazonaws.com",
		model: BEDROCK_DEFAULT_MODEL,
		smallFastModel: BEDROCK_DEFAULT_SMALL_FAST_MODEL,
	},
	azure: {
		name: "azure",
		baseUrl: "https://YOUR-RESOURCE.openai.azure.com/openai/v1",
	},
	copilot: {
		name: "copilot",
		baseUrl: "https://api.business.githubcopilot.com",
	},
	"claude-platform-aws": {
		name: "claude-platform-aws",
		baseUrl: CLAUDE_PLATFORM_DEFAULT_BASE_URL,
	},
	google: {
		name: "google",
		baseUrl: "https://generativelanguage.googleapis.com/v1beta/openai/",
	},
	"openai-compat": { name: "openai-compat", baseUrl: "" },
	openrouter: { name: "openrouter", baseUrl: "https://openrouter.ai/api/v1" },
	vercel: { name: "vercel", baseUrl: "https://ai-gateway.vercel.sh/v1" },
};

const baseUrlPlaceholders: Partial<Record<AIProviderType, string>> = {
	"openai-compat": "https://provider.example.com/v1",
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

const BEDROCK_ACCESS_KEY_PAIRED_MESSAGE =
	"Enter both access key and secret, or leave both blank to use AWS environment credentials.";

// Bedrock access keys are optional: when both are blank the server
// falls back to ambient AWS credentials (IAM role, AWS_PROFILE, IRSA,
// instance profile). Yup still requires them to be supplied as a pair
// so a half-typed rotation does not slip through.
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
		model: Yup.string().required("Model is required"),
		smallFastModel: Yup.string().required("Small-fast model is required"),
		accessKey: Yup.string().test(
			"access-key-paired",
			BEDROCK_ACCESS_KEY_PAIRED_MESSAGE,
			function (value) {
				const secret = (this.parent as { accessKeySecret?: string })
					.accessKeySecret;
				return !(credentialFilled(secret) && !credentialFilled(value));
			},
		),
		accessKeySecret: Yup.string().test(
			"access-key-secret-paired",
			BEDROCK_ACCESS_KEY_PAIRED_MESSAGE,
			function (value) {
				const accessKey = (this.parent as { accessKey?: string }).accessKey;
				return !(credentialFilled(accessKey) && !credentialFilled(value));
			},
		),
		enabled: Yup.boolean(),
	});

const makeCopilotSchema = (editing: boolean) =>
	Yup.object({
		type: Yup.string()
			.oneOf(["copilot"] as const)
			.required(),
		name: makeNameSchema(editing),
		displayName: makeDisplayNameSchema(editing),
		baseUrl: Yup.string()
			.url("Endpoint must be a valid URL")
			.matches(HTTP_SCHEME_REGEX, "Endpoint must use http or https.")
			.required("Endpoint is required"),
		enabled: Yup.boolean(),
	});

const CLAUDE_PLATFORM_ACCESS_KEY_PAIRED_MESSAGE =
	"Enter both access key and secret, or leave both blank to use the AWS default credential chain or a workspace API key.";

// Claude Platform for AWS always needs a region and workspace ID. Unlike
// Bedrock, the region is an explicit field (not parsed from the URL) so
// operators can route through a proxy base URL while still signing for the
// correct AWS region. Access keys remain optional but must be supplied as a
// pair, matching the backend's static-credential contract.
const makeClaudePlatformSchema = (editing: boolean) =>
	Yup.object({
		type: Yup.string()
			.oneOf(["claude-platform-aws"] as const)
			.required(),
		name: makeNameSchema(editing),
		displayName: makeDisplayNameSchema(editing),
		baseUrl: Yup.string()
			.url("Endpoint must be a valid URL")
			.matches(HTTP_SCHEME_REGEX, "Endpoint must use http or https.")
			.required("Endpoint is required"),
		region: Yup.string().required("Region is required"),
		workspaceId: Yup.string().required("Workspace ID is required"),
		accessKey: Yup.string().test(
			"access-key-paired",
			CLAUDE_PLATFORM_ACCESS_KEY_PAIRED_MESSAGE,
			function (value) {
				const secret = (this.parent as { accessKeySecret?: string })
					.accessKeySecret;
				return !(credentialFilled(secret) && !credentialFilled(value));
			},
		),
		accessKeySecret: Yup.string().test(
			"access-key-secret-paired",
			CLAUDE_PLATFORM_ACCESS_KEY_PAIRED_MESSAGE,
			function (value) {
				const accessKey = (this.parent as { accessKey?: string }).accessKey;
				return !(credentialFilled(accessKey) && !credentialFilled(value));
			},
		),
		roleArn: Yup.string(),
		externalId: Yup.string(),
		apiKey: Yup.string(),
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
			case "copilot":
				return makeCopilotSchema(editing);
			case "claude-platform-aws":
				return makeClaudePlatformSchema(editing);
			default:
				return Yup.object({
					type: Yup.string()
						.oneOf([
							"openai",
							"anthropic",
							"bedrock",
							"azure",
							"copilot",
							"claude-platform-aws",
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
	baseUrlPlaceholders[provider as keyof typeof baseUrlPlaceholders] ??
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
	const resolvedType = initialValues?.type ?? defaultInitialValues.type;
	const typeDefaults =
		providerDefaults[resolvedType as keyof typeof providerDefaults];

	// Seed Bedrock credentials with the mask when on file; focus clears it,
	// and a re-submitted "" tells the API mapping to keep the value.
	const maskedAccessKey = bedrockSavedAccessCredentials
		? SAVED_CREDENTIAL_MASK
		: "";
	const maskedAccessKeySecret = bedrockSavedAccessCredentials
		? SAVED_CREDENTIAL_MASK
		: "";
	// Same pattern for openai/anthropic. Prefer the API-supplied masked
	// rendering so the user sees the key's identifying suffix.
	const maskedApiKey = openAiAnthropicSavedApiKey
		? (openAiAnthropicMaskedApiKey ?? SAVED_CREDENTIAL_MASK)
		: "";

	const didSubmit = useRef(false);
	const form = useFormik<ProviderFormValues>({
		initialValues: {
			...defaultInitialValues,
			// Layer order: base defaults < type prefills < parent's initialValues.
			// Edit overrides prefills with server values; create gets them as-is.
			...(typeDefaults ?? {}),
			...initialValues,
			accessKey: maskedAccessKey,
			accessKeySecret: maskedAccessKeySecret,
			apiKey: maskedApiKey,
		},
		validationSchema: getProviderFormSchema(editing),
		validateOnMount: true,
		onSubmit: (values) => {
			didSubmit.current = true;
			return onSubmit?.(values);
		},
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

	// Restores the mask when the user leaves the field without entering
	// a new value, keeping the saved-credential appearance.
	const handleCredentialBlur = (
		field: "apiKey" | "accessKey" | "accessKeySecret",
	) => {
		const initial = form.initialValues[field];
		if (form.values[field] === "" && initial !== "") {
			void form.setFieldValue(field, initial);
		}
	};

	// When the parent's mutation finishes without an error, treat the just-
	// submitted values as the new baseline so the unsaved-changes prompt does
	// not fire on subsequent navigations. React Query reports a missing error
	// as `null`, so a truthy check covers both null and undefined.
	const previousIsLoading = useRef(isLoading);
	useEffect(() => {
		if (previousIsLoading.current && !isLoading) {
			if (didSubmit.current && !submitError) {
				// Restore credential fields to their initial masked sentinels so
				// the raw key is never left visible after a successful save.
				const remaskedValues = {
					...form.values,
					apiKey: maskedApiKey,
					accessKey: maskedAccessKey,
					accessKeySecret: maskedAccessKeySecret,
				};
				form.resetForm({ values: remaskedValues });
			}
			didSubmit.current = false;
		}
		previousIsLoading.current = isLoading;
	}, [
		isLoading,
		submitError,
		form,
		maskedApiKey,
		maskedAccessKey,
		maskedAccessKeySecret,
	]);

	const unsavedChanges = useUnsavedChangesPrompt(
		form.dirty && !form.isSubmitting,
	);

	return (
		<Form onSubmit={form.handleSubmit}>
			<FormFields>
				{Boolean(submitError) && <ErrorAlert error={submitError} />}
				{typeSelectValue !== "" &&
					typeSelectValue !== "bedrock" &&
					typeSelectValue !== "claude-platform-aws" && (
						<>
							<div className="grid grid-cols-2 items-start gap-4">
								<FormField
									required
									field={getFieldHelpers("name")}
									label="Name"
									description="Unique identifier (used in urls, can't be changed)"
									className="w-full"
									placeholder={namePlaceholder(form.values.type)}
									disabled={editing}
								/>
								<FormField
									field={getFieldHelpers("displayName")}
									label="Display name"
									description="Friendly name. Defaults to name if blank."
									className="w-full"
								/>
							</div>
							<FormField
								required
								field={getFieldHelpers("baseUrl")}
								label="Endpoint"
								description={
									typeSelectValue === "copilot" ? (
										<>
											The base URL for your Copilot tier:{" "}
											<code>https://api.individual.githubcopilot.com</code>,{" "}
											<code>https://api.business.githubcopilot.com</code>, or{" "}
											<code>https://api.enterprise.githubcopilot.com</code>.
										</>
									) : (
										"The base URL where the provider's API is hosted."
									)
								}
								className="w-full"
								placeholder={baseUrlPlaceholder(form.values.type)}
							/>
							{typeSelectValue === "copilot" ? (
								<p className="text-sm text-content-secondary m-0">
									Copilot authenticates with each user's GitHub OAuth token at
									request time, so there is no API key to configure here. This
									requires a GitHub external authentication provider to be
									configured.
								</p>
							) : (
								<CredentialField
									required
									label="API key"
									helpers={getFieldHelpers("apiKey")}
									onBlur={() => handleCredentialBlur("apiKey")}
									onFocus={() => handleCredentialFocus("apiKey")}
									autoComplete="new-password"
									placeholder={apiKeyPlaceholder(form.values.type)}
								/>
							)}
						</>
					)}

				{typeSelectValue === "bedrock" && (
					<>
						<div className="grid grid-cols-2 items-start gap-4">
							<FormField
								required
								field={getFieldHelpers("name")}
								label="Name"
								description="Unique identifier (used in urls, can't be changed)"
								className="w-full"
								placeholder={namePlaceholder(form.values.type)}
								disabled={editing}
							/>
							<FormField
								field={getFieldHelpers("displayName")}
								label="Display name"
								description="Friendly name. Defaults to name if blank."
								className="w-full"
							/>
						</div>
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
								required
								field={getFieldHelpers("model")}
								label="Model"
								className="w-full"
								placeholder={BEDROCK_DEFAULT_MODEL}
							/>
							<FormField
								required
								field={getFieldHelpers("smallFastModel")}
								label="Small-fast model"
								className="w-full"
								placeholder={BEDROCK_DEFAULT_SMALL_FAST_MODEL}
							/>
						</div>
						<p className="text-xs text-content-secondary m-0">
							Find available Bedrock model IDs in the{" "}
							<DocsLink
								size="sm"
								href={BEDROCK_MODEL_CARDS_URL}
								target="_blank"
								rel="noreferrer"
							>
								AWS Bedrock model cards
							</DocsLink>
							.
						</p>
						<div className="grid grid-cols-2 items-start gap-4">
							<CredentialField
								label="Access key"
								helpers={getFieldHelpers("accessKey")}
								onBlur={() => handleCredentialBlur("accessKey")}
								onFocus={() => handleCredentialFocus("accessKey")}
								autoComplete="new-password"
							/>
							<CredentialField
								label="Access key secret"
								helpers={getFieldHelpers("accessKeySecret")}
								onBlur={() => handleCredentialBlur("accessKeySecret")}
								onFocus={() => handleCredentialFocus("accessKeySecret")}
								autoComplete="new-password"
							/>
						</div>
						<p className="text-xs text-content-secondary m-0">
							Optional. Leave both fields blank to authenticate with the AWS
							environment (IAM role, instance profile, AWS_PROFILE).{" "}
							<DocsLink
								size="sm"
								href={docs("/ai-coder/ai-gateway/providers#amazon-bedrock")}
								target="_blank"
								rel="noreferrer"
							>
								View docs
							</DocsLink>
						</p>
						<FormField
							field={getFieldHelpers("roleArn")}
							label="Role ARN"
							className="w-full"
							placeholder="arn:aws:iam::123456789012:role/BedrockRole"
						/>
						<p className="text-xs text-content-secondary m-0">
							Optional. When a role ARN is set, the gateway assumes that role
							(using the base identity) before calling Bedrock.
						</p>
					</>
				)}

				{typeSelectValue === "claude-platform-aws" && (
					<>
						<div className="grid grid-cols-2 items-start gap-4">
							<FormField
								required
								field={getFieldHelpers("name")}
								label="Name"
								description="Unique identifier (used in urls, can't be changed)"
								className="w-full"
								placeholder={namePlaceholder(form.values.type)}
								disabled={editing}
							/>
							<FormField
								field={getFieldHelpers("displayName")}
								label="Display name"
								description="Friendly name. Defaults to name if blank."
								className="w-full"
							/>
						</div>
						<FormField
							required
							field={getFieldHelpers("baseUrl")}
							label="Endpoint"
							description={
								<>
									In the format of{" "}
									<code>
										{"https://aws-external-anthropic.{region}.api.aws"}
									</code>
									, or a proxy endpoint.
								</>
							}
							className="w-full"
							placeholder={baseUrlPlaceholder(form.values.type)}
						/>
						<div className="grid grid-cols-2 items-start gap-4">
							<FormField
								required
								field={getFieldHelpers("region")}
								label="Region"
								description="AWS region for SigV4 signing. Must match the endpoint."
								className="w-full"
								placeholder="us-east-1"
							/>
							<FormField
								required
								field={getFieldHelpers("workspaceId")}
								label="Workspace ID"
								description="Sent as the anthropic-workspace-id header."
								className="w-full"
							/>
						</div>
						<div className="grid grid-cols-2 items-start gap-4">
							<CredentialField
								label="Access key"
								helpers={getFieldHelpers("accessKey")}
								onBlur={() => handleCredentialBlur("accessKey")}
								onFocus={() => handleCredentialFocus("accessKey")}
								autoComplete="new-password"
							/>
							<CredentialField
								label="Access key secret"
								helpers={getFieldHelpers("accessKeySecret")}
								onBlur={() => handleCredentialBlur("accessKeySecret")}
								onFocus={() => handleCredentialFocus("accessKeySecret")}
								autoComplete="new-password"
							/>
						</div>
						<p className="text-xs text-content-secondary m-0">
							Optional. Leave both blank to use the AWS default credential chain
							(IAM role, instance profile, AWS_PROFILE). When editing, leave
							blank to keep saved credentials.{" "}
							<DocsLink
								size="sm"
								href={docs(
									"/ai-coder/ai-gateway/providers#claude-platform-for-aws",
								)}
								target="_blank"
								rel="noreferrer"
							>
								View docs
							</DocsLink>
						</p>
						<div className="grid grid-cols-2 items-start gap-4">
							<FormField
								field={getFieldHelpers("roleArn")}
								label="Role ARN"
								className="w-full"
								placeholder="arn:aws:iam::123456789012:role/ClaudePlatformRole"
							/>
							<FormField
								field={getFieldHelpers("externalId")}
								label="External ID"
								className="w-full"
							/>
						</div>
						<p className="text-xs text-content-secondary m-0">
							Optional. When a role ARN is set, the gateway assumes that role
							(using the base identity) before calling Claude Platform. External
							ID is passed when assuming the role.
						</p>
						<CredentialField
							label="API key"
							helpers={getFieldHelpers("apiKey")}
							onBlur={() => handleCredentialBlur("apiKey")}
							onFocus={() => handleCredentialFocus("apiKey")}
							autoComplete="new-password"
						/>
						<p className="text-xs text-content-secondary m-0">
							Optional. A Claude Platform workspace API key, sent as x-api-key.
							It takes precedence over AWS credentials. When editing, leave
							blank to keep the saved key.
						</p>
					</>
				)}

				<div className="flex justify-end gap-4">
					<Link to="/ai/settings/providers">
						<Button variant="outline" type="button">
							Cancel
						</Button>
					</Link>
					<Button
						disabled={isLoading || !form.isValid || (editing && !form.dirty)}
						type="submit"
					>
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
