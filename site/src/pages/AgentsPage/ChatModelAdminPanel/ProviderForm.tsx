import { getErrorMessage } from "api/errors";
import type * as TypesGen from "api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "components/Alert/Alert";
import { Button } from "components/Button/Button";
import { Input } from "components/Input/Input";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { ChevronLeftIcon, InfoIcon, Loader2Icon } from "lucide-react";
import {
	type CSSProperties,
	type FC,
	type FormEvent,
	type ReactNode,
	useEffect,
	useId,
	useState,
} from "react";
import { toast } from "sonner";
import { formatProviderLabel } from "../modelOptions";
import { readOptionalString } from "./helpers";
import { ProviderIcon } from "./ProviderIcon";

// Sentinel value used to represent an existing API key that the
// backend won't reveal. If the user hasn't touched the field,
// we know nothing changed.
const API_KEY_PLACEHOLDER = "••••••••••••••••";

type ProviderFormProps = {
	provider: string;
	providerConfig: TypesGen.ChatProviderConfig | undefined;
	baseURL: string;
	isEnvPreset: boolean;
	providerConfigsUnavailable: boolean;
	isProviderMutationPending: boolean;
	onCreateProvider: (
		req: TypesGen.CreateChatProviderConfigRequest,
	) => Promise<unknown>;
	onUpdateProvider: (
		providerConfigId: string,
		req: TypesGen.UpdateChatProviderConfigRequest,
	) => Promise<unknown>;
	onDeleteProvider?: (providerConfigId: string) => Promise<void>;
	onBack?: () => void;
};

export const ProviderForm: FC<ProviderFormProps> = ({
	provider,
	providerConfig,
	baseURL,
	isEnvPreset,
	providerConfigsUnavailable,
	isProviderMutationPending,
	onCreateProvider,
	onUpdateProvider,
	onDeleteProvider,
	onBack,
}) => {
	const apiKeyInputId = useId();
	const baseURLInputId = useId();
	const [confirmingDelete, setConfirmingDelete] = useState(false);

	const [displayName, setDisplayName] = useState("");
	const [apiKey, setApiKey] = useState("");
	const [apiKeyTouched, setApiKeyTouched] = useState(false);
	const [baseURLValue, setBaseURLValue] = useState("");

	useEffect(() => {
		setDisplayName(readOptionalString(providerConfig?.display_name) ?? "");
		setApiKey(providerConfig?.has_api_key ? API_KEY_PLACEHOLDER : "");
		setApiKeyTouched(false);
		setBaseURLValue(baseURL);
		setConfirmingDelete(false);
	}, [providerConfig, baseURL]);

	const isAPIKeyEnvManaged = isEnvPreset && !providerConfig;
	const requiresAPIKey = !providerConfig && !isAPIKeyEnvManaged;
	const currentDisplayName =
		readOptionalString(providerConfig?.display_name) ?? "";
	const currentBaseURL = baseURL.trim();
	const effectiveApiKey =
		apiKeyTouched && apiKey !== API_KEY_PLACEHOLDER ? apiKey.trim() : "";
	const isDirty =
		displayName.trim() !== currentDisplayName ||
		baseURLValue.trim() !== currentBaseURL ||
		effectiveApiKey !== "";
	const isDisabled = providerConfigsUnavailable || isProviderMutationPending;
	const canSave =
		!providerConfigsUnavailable &&
		!isProviderMutationPending &&
		!isAPIKeyEnvManaged &&
		isDirty &&
		(!requiresAPIKey || effectiveApiKey.length > 0);

	const handleSubmit = async (event: FormEvent) => {
		event.preventDefault();
		if (
			providerConfigsUnavailable ||
			isProviderMutationPending ||
			isAPIKeyEnvManaged
		) {
			return;
		}

		const trimmedDisplayName = displayName.trim();
		const trimmedBaseURL = baseURLValue.trim();

		try {
			if (providerConfig) {
				const req: TypesGen.UpdateChatProviderConfigRequest = {
					...(trimmedDisplayName !== currentDisplayName && {
						display_name: trimmedDisplayName,
					}),
					...(effectiveApiKey && { api_key: effectiveApiKey }),
					...(trimmedBaseURL !== currentBaseURL && {
						base_url: trimmedBaseURL,
					}),
				};

				if (!req.display_name && !req.api_key && !req.base_url) {
					return;
				}

				await onUpdateProvider(providerConfig.id, req);
			} else {
				if (!effectiveApiKey) {
					return;
				}

				const req: TypesGen.CreateChatProviderConfigRequest = {
					provider,
					api_key: effectiveApiKey,
					...(trimmedDisplayName && {
						display_name: trimmedDisplayName,
					}),
					...(trimmedBaseURL && { base_url: trimmedBaseURL }),
				};

				await onCreateProvider(req);
			}

			setApiKey(providerConfig?.has_api_key ? API_KEY_PLACEHOLDER : "");
			setApiKeyTouched(false);
		} catch (error) {
			toast.error(
				getErrorMessage(error, "Failed to save provider configuration."),
			);
		}
	};

	const handleApiKeyFocus = () => {
		// Clear the placeholder on first focus so the user starts
		// with a blank field and password managers don't replace it.
		if (!apiKeyTouched && apiKey === API_KEY_PLACEHOLDER) {
			setApiKey("");
			setApiKeyTouched(true);
		}
	};

	return (
		<div className="border-t border-border px-5 py-4">
			<div className="flex min-h-full flex-col">
				{onBack && (
					<button
						type="button"
						onClick={onBack}
						className="mb-4 inline-flex cursor-pointer items-center gap-0.5 border-0 bg-transparent p-0 text-sm text-content-secondary transition-colors hover:text-content-primary"
					>
						<ChevronLeftIcon className="h-4 w-4" />
						Back
					</button>
				)}

				<div className="flex items-center gap-3">
					<ProviderIcon provider={provider} className="h-8 w-8" />
					<div className="min-w-0 flex-1">
						<input
							type="text"
							value={displayName || formatProviderLabel(provider)}
							onChange={(e) => setDisplayName(e.target.value)}
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
							This provider key is configured from deployment environment
							settings and cannot be edited in this UI.
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
							<ProviderField
								label="API Key"
								htmlFor={apiKeyInputId}
								required={!providerConfig}
								description="Secret key used to authenticate requests to this provider."
							>
								<Input
									id={apiKeyInputId}
									name="provider_api_token"
									type="text"
									autoComplete="off"
									data-1p-ignore
									data-lpignore="true"
									data-form-type="other"
									data-bwignore
									style={{ WebkitTextSecurity: "disc" } as CSSProperties}
									className="h-9 font-mono text-[13px]"
									placeholder="sk-..."
									value={apiKey}
									onFocus={handleApiKeyFocus}
									onChange={(e) => {
										setApiKey(e.target.value);
										setApiKeyTouched(true);
									}}
									disabled={isDisabled}
								/>
							</ProviderField>

							<ProviderField
								label="Base URL"
								htmlFor={baseURLInputId}
								description="Custom endpoint for this provider. Leave empty to use the default."
							>
								<Input
									id={baseURLInputId}
									name="provider_base_url"
									className="h-9 text-[13px]"
									placeholder="https://api.example.com/v1"
									autoComplete="off"
									value={baseURLValue}
									onChange={(e) => setBaseURLValue(e.target.value)}
									disabled={isDisabled}
								/>
							</ProviderField>
						</div>

						<div className="mt-auto pt-6">
							<hr className="mb-4 border-0 border-t border-solid border-border" />
							{confirmingDelete && providerConfig && onDeleteProvider ? (
								<div className="flex items-center gap-3">
									<p className="m-0 flex-1 text-sm text-content-secondary">
										Are you sure? This action is irreversible.
									</p>
									<div className="flex shrink-0 items-center gap-2">
										<Button
											variant="outline"
											size="lg"
											type="button"
											onClick={() => setConfirmingDelete(false)}
											disabled={isProviderMutationPending}
										>
											Cancel
										</Button>
										<Button
											variant="destructive"
											size="lg"
											type="button"
											disabled={isProviderMutationPending}
											onClick={() => void onDeleteProvider(providerConfig.id)}
										>
											{isProviderMutationPending && (
												<Loader2Icon className="h-4 w-4 animate-spin" />
											)}
											Delete provider
										</Button>
									</div>
								</div>
							) : (
								<div className="flex items-center justify-between">
									{providerConfig && onDeleteProvider ? (
										<Button
											variant="outline"
											size="lg"
											type="button"
											className="text-content-secondary hover:border-border-destructive hover:text-content-destructive"
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
											<Loader2Icon className="h-4 w-4 animate-spin" />
										)}
										{providerConfig ? "Save changes" : "Create provider config"}
									</Button>
								</div>
							)}
						</div>
					</form>
				)}
			</div>
		</div>
	);
};

type ProviderFieldProps = {
	label: string;
	htmlFor: string;
	required?: boolean;
	description?: string;
	children: ReactNode;
};

const ProviderField: FC<ProviderFieldProps> = ({
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
