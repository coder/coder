import { InfoIcon } from "lucide-react";
import {
	type CSSProperties,
	type FC,
	type FormEvent,
	type ReactNode,
	useId,
	useState,
} from "react";
import { useNavigate } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { formatProviderLabel } from "../../utils/modelOptions";
import { BackButton } from "../BackButton";
import { ConfirmDeleteDialog } from "../ConfirmDeleteDialog";
import type {
	CreateProviderResult,
	ProviderState,
} from "./ChatModelAdminPanel";
import { getProviderBaseURLPlaceholder, readOptionalString } from "./helpers";
import { ProviderIcon } from "./ProviderIcon";

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
	) => Promise<CreateProviderResult>;
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
	const navigate = useNavigate();
	const { provider, providerConfig, baseURL, isEnvPreset } = providerState;

	const apiKeyInputId = useId();
	const baseURLInputId = useId();

	const baseURLPlaceholder = getProviderBaseURLPlaceholder(provider);

	// Initial values are snapshotted when the provider config changes
	// so we can detect dirty state.
	const [initialValues] = useState(() => ({
		displayName: readOptionalString(providerConfig?.display_name) ?? "",
		baseURL,
	}));

	const [displayName, setDisplayName] = useState(initialValues.displayName);
	const [apiKey, setApiKey] = useState(
		providerState.hasManagedAPIKey ? API_KEY_PLACEHOLDER : "",
	);
	const [apiKeyTouched, setApiKeyTouched] = useState(false);
	const [apiKeyModified, setApiKeyModified] = useState(false);
	const [baseURLValue, setBaseURLValue] = useState(initialValues.baseURL);
	const [confirmingDelete, setConfirmingDelete] = useState(false);

	const isBedrockProvider = provider === "bedrock";
	const isAPIKeyEnvManaged = isEnvPreset && !providerConfig;
	const requiresAPIKey =
		!providerState.allowUserAPIKey &&
		!isBedrockProvider &&
		!providerState.hasManagedAPIKey;

	const effectiveApiKey =
		apiKeyTouched && apiKey !== API_KEY_PLACEHOLDER ? apiKey : "";
	const hasTypedAPIKey = effectiveApiKey.length > 0;
	const hasAPIKeyWhitespace =
		hasTypedAPIKey && effectiveApiKey.trim() !== effectiveApiKey;
	// Clearing a saved provider-scoped key switches the provider to
	// BYOK-only behavior.
	const isClearingAPIKey =
		providerState.hasManagedAPIKey && apiKeyModified && effectiveApiKey === "";
	const hasPendingAPIKeyChange =
		!isBedrockProvider && (hasTypedAPIKey || isClearingAPIKey);
	const shouldCreateAPIKey = !isBedrockProvider && hasTypedAPIKey;
	const apiKeyDescription = isBedrockProvider
		? "AWS credentials for Bedrock are managed in AI settings."
		: "Secret key used to authenticate requests to this provider.";
	const baseURLDescription = isBedrockProvider
		? "Bedrock runtime endpoint. Use the AWS region for the models this provider should call."
		: "Endpoint used to call this provider.";
	const apiKeyPlaceholder = isBedrockProvider
		? "Managed in AI settings"
		: "sk-...";
	const deleteProviderDescription =
		"Are you sure you want to delete this provider? The provider will be " +
		"disabled and hidden from new model configuration. Existing model " +
		"configs that reference it remain saved but cannot run until updated.";
	const hasNewProviderConfiguration = !providerConfig;

	const requiresBedrockAISettings = isBedrockProvider && !providerConfig;

	const isDirty =
		displayName.trim() !== initialValues.displayName ||
		hasPendingAPIKeyChange ||
		baseURLValue.trim() !== initialValues.baseURL.trim() ||
		hasNewProviderConfiguration;

	const hasBaseURL = baseURLValue.trim().length > 0;
	const canSave =
		!providerConfigsUnavailable &&
		!isProviderMutationPending &&
		!isAPIKeyEnvManaged &&
		!requiresBedrockAISettings &&
		isDirty &&
		hasBaseURL &&
		!hasAPIKeyWhitespace &&
		(!requiresAPIKey || hasTypedAPIKey);
	const canAddModel =
		Boolean(providerConfig) &&
		(providerState.hasEffectiveAPIKey ||
			providerConfig?.allow_user_api_key === true);

	const handleAddModel = () => {
		const params = new URLSearchParams({ newModel: providerState.key });
		navigate(`/agents/settings/models?${params.toString()}`, {
			state: { pushed: true },
		});
	};

	const handleSubmit = async (event: FormEvent) => {
		event.preventDefault();
		if (
			providerConfigsUnavailable ||
			isProviderMutationPending ||
			isAPIKeyEnvManaged ||
			requiresBedrockAISettings ||
			!hasBaseURL ||
			hasAPIKeyWhitespace
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
			};

			if (Object.keys(req).length === 0) {
				return;
			}

			try {
				await onUpdateProvider(providerConfig.id, req);
			} catch {
				return;
			}
		} else {
			const req: TypesGen.CreateChatProviderConfigRequest = {
				provider,
				base_url: trimmedBaseURL,
				...(shouldCreateAPIKey && { api_key: effectiveApiKey }),
				...(trimmedDisplayName && {
					display_name: trimmedDisplayName,
				}),
			};

			try {
				await onCreateProvider(req);
			} catch {
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
			<BackButton onClick={onBack} />
			<div className="flex items-center gap-3">
				<ProviderIcon provider={provider} className="size-8" />
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
						<InfoIcon className="size-4 shrink-0 cursor-help text-content-secondary" />
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
						{requiresBedrockAISettings && (
							<Alert severity="info">
								<AlertTitle>Configure AWS Bedrock in AI settings</AlertTitle>
								<AlertDescription>
									Bedrock providers require AWS region and credential settings.
									Create the provider in AI settings, then return here to add
									models.
								</AlertDescription>
								<Button
									type="button"
									variant="outline"
									size="sm"
									onClick={() => navigate("/ai/settings/add?type=bedrock")}
								>
									Open AI settings
								</Button>
							</Alert>
						)}
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
									disabled={isDisabled || isBedrockProvider}
								/>
								{hasAPIKeyWhitespace && (
									<p className="m-0 text-xs text-content-destructive">
										API key must not contain leading or trailing whitespace.
									</p>
								)}
							</div>
						</ProviderField>

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
								required
								autoComplete="off"
								value={baseURLValue}
								onChange={(event) => setBaseURLValue(event.target.value)}
								disabled={isDisabled}
							/>
						</ProviderField>
					</div>
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
							<div className="flex items-center gap-2">
								{canAddModel && (
									<Button size="lg" type="button" onClick={handleAddModel}>
										Add model
									</Button>
								)}
								<Button
									size="lg"
									type="submit"
									variant={canAddModel ? "outline" : undefined}
									disabled={!canSave}
								>
									{isProviderMutationPending && (
										<Spinner className="h-4 w-4" loading />
									)}
									{providerConfig ? "Save changes" : "Create provider config"}
								</Button>
							</div>
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
