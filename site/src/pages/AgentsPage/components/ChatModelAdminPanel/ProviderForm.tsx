import { InfoIcon } from "lucide-react";
import {
	type CSSProperties,
	type FC,
	type FormEvent,
	type ReactNode,
	useId,
	useState,
} from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { formatProviderLabel } from "../../utils/modelOptions";
import { BackButton } from "../BackButton";
import { ConfirmDeleteDialog } from "../ConfirmDeleteDialog";
import type { ProviderState } from "./ChatModelAdminPanel";
import { readOptionalString } from "./helpers";
import { ProviderIcon } from "./ProviderIcon";
import { normalizeProviderPolicyDefaults } from "./providerPolicyDefaults";

// Sentinel value used to represent an existing API key that the
// backend will not reveal. If the user has not touched the field,
// we know nothing changed.
const API_KEY_PLACEHOLDER = "••••••••••••••••";

interface ProviderFormProps {
	providerState: ProviderState;
	providerConfigsUnavailable: boolean;
	isProviderMutationPending: boolean;
	onCreateProvider: (
		req: TypesGen.CreateChatProviderConfigRequest,
	) => Promise<unknown>;
	onUpdateProvider: (
		providerConfigId: string,
		req: TypesGen.UpdateChatProviderConfigRequest,
	) => Promise<unknown>;
	onDeleteProvider: (providerConfigId: string) => Promise<void>;
	onBack: () => void;
}

export const ProviderForm: FC<ProviderFormProps> = ({
	providerState,
	providerConfigsUnavailable,
	isProviderMutationPending,
	onCreateProvider,
	onUpdateProvider,
	onDeleteProvider,
	onBack,
}) => {
	const { provider, providerConfig, baseURL, isEnvPreset } = providerState;

	const apiKeyInputId = useId();
	const baseURLInputId = useId();

	// Providers backed by the OpenAI SDK expect /v1 in the base
	// URL, while others (e.g. Anthropic) do not.
	const baseURLPlaceholder =
		provider === "anthropic" || provider === "bedrock" || provider === "google"
			? "https://api.example.com"
			: "https://api.example.com/v1";

	const normalizedProviderConfig = providerConfig
		? normalizeProviderPolicyDefaults(providerConfig)
		: undefined;

	// Initial values are snapshotted when the provider config changes
	// so we can detect dirty state.
	const [initialValues] = useState(() => ({
		displayName: readOptionalString(providerConfig?.display_name) ?? "",
		baseURL,
		centralAPIKeyEnabled:
			normalizedProviderConfig?.central_api_key_enabled ?? true,
		allowUserAPIKey: normalizedProviderConfig?.allow_user_api_key ?? false,
		allowCentralAPIKeyFallback:
			normalizedProviderConfig?.allow_central_api_key_fallback ?? false,
	}));

	const [displayName, setDisplayName] = useState(initialValues.displayName);
	const [apiKey, setApiKey] = useState(
		providerState.hasManagedAPIKey ? API_KEY_PLACEHOLDER : "",
	);
	const [apiKeyTouched, setApiKeyTouched] = useState(false);
	const [apiKeyModified, setApiKeyModified] = useState(false);
	const [baseURLValue, setBaseURLValue] = useState(initialValues.baseURL);
	const [centralAPIKeyEnabled, setCentralAPIKeyEnabled] = useState(
		initialValues.centralAPIKeyEnabled,
	);
	const [allowUserAPIKey, setAllowUserAPIKey] = useState(
		initialValues.allowUserAPIKey,
	);
	const [allowCentralAPIKeyFallback, setAllowCentralAPIKeyFallback] = useState(
		initialValues.allowCentralAPIKeyFallback,
	);
	const [confirmingDelete, setConfirmingDelete] = useState(false);

	const isBedrockProvider = provider === "bedrock";
	const isAPIKeyEnvManaged = isEnvPreset && !providerConfig;
	const shouldShowAPIKeyField = centralAPIKeyEnabled;
	const shouldShowFallbackToggle = centralAPIKeyEnabled && allowUserAPIKey;
	const effectiveInitialFallback =
		initialValues.centralAPIKeyEnabled &&
		initialValues.allowUserAPIKey &&
		initialValues.allowCentralAPIKeyFallback;
	const effectiveFallback =
		shouldShowFallbackToggle && allowCentralAPIKeyFallback;
	// Most providers require a stored deployment key whenever central-key
	// usage is enabled and there is no saved key yet. Bedrock can also use
	// ambient AWS credentials from the Coder server, so its API key stays
	// optional.
	const requiresAPIKey =
		!isAPIKeyEnvManaged &&
		!isBedrockProvider &&
		centralAPIKeyEnabled &&
		!providerState.hasManagedAPIKey;

	const effectiveApiKey =
		apiKeyTouched && apiKey !== API_KEY_PLACEHOLDER ? apiKey.trim() : "";
	const hasTypedAPIKey = effectiveApiKey.length > 0;
	// Clearing a saved Bedrock bearer token switches the provider back
	// to ambient AWS credentials, so updates must send an explicit
	// empty string.
	const isClearingBedrockAPIKey =
		isBedrockProvider &&
		providerState.hasManagedAPIKey &&
		apiKeyModified &&
		effectiveApiKey === "";
	const hasPendingAPIKeyChange =
		(centralAPIKeyEnabled && hasTypedAPIKey) || isClearingBedrockAPIKey;
	const shouldCreateAPIKey = centralAPIKeyEnabled && hasTypedAPIKey;
	const hasCredentialSource = centralAPIKeyEnabled || allowUserAPIKey;
	const apiKeyDescription = isBedrockProvider
		? "Bearer token for Bedrock authentication. Leave empty to use ambient AWS credentials."
		: "Secret key used to authenticate requests to this provider.";
	const baseURLDescription = isBedrockProvider
		? "Optional. Overrides the Bedrock runtime endpoint. Set AWS_REGION on the Coder server to select the target region."
		: "Custom endpoint for this provider. Leave empty to use the default.";
	const apiKeyPlaceholder = isBedrockProvider ? "Enter bearer token" : "sk-...";
	const deleteProviderDescription = normalizedProviderConfig?.allow_user_api_key
		? "Are you sure you want to delete this provider? Any personal API " +
			"keys that users have saved for this provider will also be " +
			"permanently deleted. This action is irreversible."
		: "Are you sure you want to delete this provider? This action is irreversible.";
	// New Bedrock providers can be saved immediately with ambient AWS
	// credentials, even before any fields differ from their defaults.
	const hasNewBedrockAmbientConfiguration =
		isBedrockProvider && !providerConfig && centralAPIKeyEnabled;

	const isDirty =
		displayName.trim() !== initialValues.displayName ||
		hasPendingAPIKeyChange ||
		baseURLValue.trim() !== initialValues.baseURL.trim() ||
		centralAPIKeyEnabled !== initialValues.centralAPIKeyEnabled ||
		allowUserAPIKey !== initialValues.allowUserAPIKey ||
		effectiveFallback !== effectiveInitialFallback ||
		hasNewBedrockAmbientConfiguration;

	const canSave =
		!providerConfigsUnavailable &&
		!isProviderMutationPending &&
		!isAPIKeyEnvManaged &&
		isDirty &&
		hasCredentialSource &&
		(!requiresAPIKey || hasTypedAPIKey);

	const handleSubmit = async (event: FormEvent) => {
		event.preventDefault();
		if (
			providerConfigsUnavailable ||
			isProviderMutationPending ||
			isAPIKeyEnvManaged ||
			!hasCredentialSource
		) {
			return;
		}

		if (requiresAPIKey && !hasTypedAPIKey) {
			return;
		}

		const trimmedDisplayName = displayName.trim();
		const trimmedBaseURL = baseURLValue.trim();

		if (providerConfig) {
			const currentDisplayName =
				readOptionalString(providerConfig.display_name) ?? "";
			const currentBaseURL = baseURL.trim();
			const req: TypesGen.UpdateChatProviderConfigRequest = {
				...(trimmedDisplayName !== currentDisplayName && {
					display_name: trimmedDisplayName,
				}),
				...(hasPendingAPIKeyChange && { api_key: effectiveApiKey }),
				...(trimmedBaseURL !== currentBaseURL && {
					base_url: trimmedBaseURL,
				}),
				...(centralAPIKeyEnabled !== initialValues.centralAPIKeyEnabled && {
					central_api_key_enabled: centralAPIKeyEnabled,
				}),
				...(allowUserAPIKey !== initialValues.allowUserAPIKey && {
					allow_user_api_key: allowUserAPIKey,
				}),
				...(effectiveFallback !== effectiveInitialFallback && {
					allow_central_api_key_fallback: effectiveFallback,
				}),
			};

			if (Object.keys(req).length === 0) {
				return;
			}

			try {
				await onUpdateProvider(providerConfig.id, req);
			} catch {
				// Error is surfaced via the mutation's error state
				// in ChatModelAdminPanel, no toast needed.
				return;
			}
		} else {
			const req: TypesGen.CreateChatProviderConfigRequest = {
				provider,
				...(shouldCreateAPIKey && { api_key: effectiveApiKey }),
				central_api_key_enabled: centralAPIKeyEnabled,
				allow_user_api_key: allowUserAPIKey,
				allow_central_api_key_fallback: effectiveFallback,
				...(trimmedDisplayName && {
					display_name: trimmedDisplayName,
				}),
				...(trimmedBaseURL && { base_url: trimmedBaseURL }),
			};

			try {
				await onCreateProvider(req);
			} catch {
				// Error is surfaced via the mutation's error state
				// in ChatModelAdminPanel, no toast needed.
				return;
			}
		}

		setApiKeyTouched(false);
		setApiKeyModified(false);
		setApiKey(API_KEY_PLACEHOLDER);
	};

	const handleApiKeyFocus = () => {
		// Clear the placeholder on first focus so the user starts
		// with a blank field and Chrome does not try to autofill.
		if (!apiKeyTouched && apiKey === API_KEY_PLACEHOLDER) {
			setApiKey("");
			setApiKeyTouched(true);
		}
	};

	const isDisabled = providerConfigsUnavailable || isProviderMutationPending;

	return (
		<div className="flex min-h-full flex-col">
			{/* Back */}
			<BackButton onClick={onBack} />
			{/* Provider header, editable name */}
			<div className="flex items-center gap-3">
				<ProviderIcon provider={provider} className="h-8 w-8" />
				<div className="min-w-0 flex-1">
					<input
						type="text"
						value={displayName || formatProviderLabel(provider)}
						onChange={(event) => setDisplayName(event.target.value)}
						disabled={isDisabled || isAPIKeyEnvManaged}
						className="m-0 w-full border-0 bg-transparent p-0 text-lg font-medium text-content-primary outline-none placeholder:text-content-secondary focus:ring-0"
						placeholder={formatProviderLabel(provider)}
					/>
				</div>
				<Tooltip>
					<TooltipTrigger asChild>
						<InfoIcon className="h-4 w-4 shrink-0 cursor-help text-content-secondary" />
					</TooltipTrigger>
					<TooltipContent>
						Uses the {formatProviderLabel(provider)} API specification
					</TooltipContent>
				</Tooltip>
			</div>
			<hr className="my-4 border-0 border-t border-solid border-border" />
			{isAPIKeyEnvManaged ? (
				<Alert severity="info">
					<AlertTitle>API key managed by environment variable</AlertTitle>
					<AlertDescription>
						This provider key is configured from deployment environment settings
						and cannot be edited in this UI.
					</AlertDescription>
				</Alert>
			) : (
				<form
					className="flex flex-1 flex-col"
					onSubmit={(event) => void handleSubmit(event)}
					autoComplete="off"
					data-form-type="other"
				>
					<div className="space-y-5">
						{shouldShowAPIKeyField && (
							<ProviderField
								label="API Key"
								htmlFor={apiKeyInputId}
								required={requiresAPIKey}
								description={apiKeyDescription}
							>
								<div className="space-y-1.5">
									<Input
										id={apiKeyInputId}
										name="provider_api_token"
										type="password"
										autoComplete="off"
										data-1p-ignore
										data-lpignore="true"
										data-form-type="other"
										data-bwignore
										style={{ WebkitTextSecurity: "disc" } as CSSProperties}
										className="h-9 font-mono text-[13px]"
										placeholder={apiKeyPlaceholder}
										required={requiresAPIKey}
										value={apiKey}
										onFocus={handleApiKeyFocus}
										onChange={(event) => {
											setApiKey(event.target.value);
											setApiKeyTouched(true);
											setApiKeyModified(true);
										}}
										disabled={isDisabled}
									/>
									{isBedrockProvider &&
										providerState.hasManagedAPIKey &&
										!isDisabled &&
										(!apiKeyModified || apiKey !== "") && (
											<div className="flex justify-end">
												<button
													type="button"
													className="appearance-none border-0 bg-transparent p-0 text-xs text-content-link hover:cursor-pointer hover:underline"
													onClick={() => {
														setApiKey("");
														setApiKeyTouched(true);
														setApiKeyModified(true);
													}}
												>
													Clear stored token
												</button>
											</div>
										)}
								</div>
							</ProviderField>
						)}

						<ProviderField
							label="Base URL"
							htmlFor={baseURLInputId}
							description={baseURLDescription}
						>
							<Input
								id={baseURLInputId}
								name="provider_base_url"
								className="h-9 text-[13px]"
								placeholder={baseURLPlaceholder}
								autoComplete="off"
								value={baseURLValue}
								onChange={(event) => setBaseURLValue(event.target.value)}
								disabled={isDisabled}
							/>
						</ProviderField>

						<div className="space-y-3 rounded-lg border border-solid border-border/70 bg-surface-secondary/30 p-4">
							<div className="space-y-1">
								<h3 className="m-0 text-[13px] font-semibold text-content-primary">
									Key policy
								</h3>
								<p className="m-0 text-xs text-content-secondary">
									Control which credential sources this provider can use.
								</p>
							</div>
							<div className="space-y-3">
								<ProviderToggleField
									label="Central API key"
									description="Use a deployment-managed API key for this provider"
									checked={centralAPIKeyEnabled}
									onCheckedChange={setCentralAPIKeyEnabled}
									disabled={isDisabled}
								/>
								<ProviderToggleField
									label="Allow user API keys"
									description="Let users provide their own API keys for this provider"
									checked={allowUserAPIKey}
									onCheckedChange={setAllowUserAPIKey}
									disabled={isDisabled}
								/>
								{shouldShowFallbackToggle && (
									<ProviderToggleField
										label="Use central key as fallback"
										description="When a user has not saved a personal key, fall back to the central API key"
										checked={effectiveFallback}
										onCheckedChange={setAllowCentralAPIKeyFallback}
										disabled={isDisabled}
									/>
								)}
							</div>
							{!hasCredentialSource && (
								<p className="m-0 text-xs text-content-destructive">
									At least one credential source must be enabled
								</p>
							)}
						</div>
					</div>

					{/* Footer, pushed to bottom */}
					<div className="mt-auto pt-6">
						<hr className="mb-4 border-0 border-t border-solid border-border" />
						<div className="flex items-center justify-between">
							{providerConfig ? (
								<Button
									variant="outline"
									size="lg"
									type="button"
									className="text-content-secondary hover:text-content-destructive hover:border-border-destructive"
									disabled={isDisabled}
									onClick={() => setConfirmingDelete(true)}
								>
									Delete
								</Button>
							) : (
								<div />
							)}
							<Button size="lg" type="submit" disabled={!canSave}>
								{isProviderMutationPending && (
									<Spinner className="h-4 w-4" loading />
								)}
								{providerConfig ? "Save changes" : "Create provider config"}
							</Button>
						</div>
					</div>
				</form>
			)}
			{providerConfig && (
				<ConfirmDeleteDialog
					entity="provider"
					description={deleteProviderDescription}
					onConfirm={() => void onDeleteProvider(providerConfig.id)}
					isPending={isProviderMutationPending}
					open={confirmingDelete}
					onOpenChange={(open) => !open && setConfirmingDelete(false)}
				/>
			)}
		</div>
	);
};

interface ProviderToggleFieldProps {
	label: string;
	description: string;
	checked: boolean;
	onCheckedChange: (checked: boolean) => void;
	disabled?: boolean;
}

const ProviderToggleField: FC<ProviderToggleFieldProps> = ({
	label,
	description,
	checked,
	onCheckedChange,
	disabled,
}) => {
	const labelId = useId();
	const descriptionId = useId();

	return (
		<div className="flex items-start justify-between gap-4">
			<div className="min-w-0 space-y-1">
				<p
					id={labelId}
					className="m-0 text-sm font-medium text-content-primary"
				>
					{label}
				</p>
				<p id={descriptionId} className="m-0 text-xs text-content-secondary">
					{description}
				</p>
			</div>
			<Switch
				checked={checked}
				onCheckedChange={onCheckedChange}
				disabled={disabled}
				aria-labelledby={labelId}
				aria-describedby={descriptionId}
			/>
		</div>
	);
};

// Field wrapper.
interface ProviderFieldProps {
	label: string;
	htmlFor?: string;
	required?: boolean;
	description?: string;
	children: ReactNode;
}

export const ProviderField: FC<ProviderFieldProps> = ({
	label,
	htmlFor,
	required,
	description,
	children,
}) => (
	<div className="grid gap-1.5">
		<div className="flex items-baseline gap-1.5">
			<label
				htmlFor={htmlFor}
				className="text-sm font-medium text-content-primary"
			>
				{label}
			</label>
			{required && (
				<span className="text-xs font-bold text-content-destructive">*</span>
			)}
		</div>
		{description && (
			<p className="m-0 text-xs text-content-secondary">{description}</p>
		)}
		{children}
	</div>
);
