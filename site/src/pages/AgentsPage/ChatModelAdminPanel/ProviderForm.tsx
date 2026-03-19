import { listProviderModels } from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "components/Alert/Alert";
import { Button } from "components/Button/Button";
import { Input } from "components/Input/Input";
import { Spinner } from "components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import {
	CheckCircleIcon,
	ChevronLeftIcon,
	InfoIcon,
	XCircleIcon,
} from "lucide-react";
import { type FC, type FormEvent, useId, useState } from "react";
import { useMutation } from "react-query";
import { formatProviderLabel } from "../modelOptions";
import type { ProviderState } from "./ChatModelAdminPanel";
import { readOptionalString } from "./helpers";
import { ProviderIcon } from "./ProviderIcon";

// Sentinel value used to represent an existing API key that the
// backend won't reveal. If the user hasn't touched the field,
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

	// Initial values are snapshotted when the provider config changes
	// so we can detect dirty state.
	const [initialValues] = useState(() => ({
		displayName: readOptionalString(providerConfig?.display_name) ?? "",
		baseURL: baseURL,
	}));

	const [displayName, setDisplayName] = useState(initialValues.displayName);
	const [apiKey, setApiKey] = useState(
		providerState.hasManagedAPIKey ? API_KEY_PLACEHOLDER : "",
	);
	const [apiKeyTouched, setApiKeyTouched] = useState(false);
	const [baseURLValue, setBaseURLValue] = useState(initialValues.baseURL);
	const [confirmingDelete, setConfirmingDelete] = useState(false);

	const isAPIKeyEnvManaged = isEnvPreset && !providerConfig;
	const requiresAPIKey = !providerConfig && !isAPIKeyEnvManaged;

	// The actual API key value to submit — ignore the placeholder.
	const effectiveApiKey =
		apiKeyTouched && apiKey !== API_KEY_PLACEHOLDER ? apiKey.trim() : "";

	// Dirty detection: has anything changed from the initial state?
	const isDirty =
		displayName.trim() !== initialValues.displayName ||
		effectiveApiKey !== "" ||
		baseURLValue.trim() !== initialValues.baseURL.trim();

	const canSave =
		!providerConfigsUnavailable &&
		!isProviderMutationPending &&
		!isAPIKeyEnvManaged &&
		isDirty &&
		(!requiresAPIKey || effectiveApiKey);

	// ── Test connection (list models) ────────────────────────────
	const testMutation = useMutation(listProviderModels());

	// The test button is available when there is a usable API key —
	// either the user typed one in, or an existing config has one saved.
	const canTest =
		!providerConfigsUnavailable &&
		!isAPIKeyEnvManaged &&
		(effectiveApiKey !== "" || providerState.hasManagedAPIKey);

	const handleTest = () => {
		const trimmedBaseURL = baseURLValue.trim();
		if (providerConfig && !effectiveApiKey) {
			// Test an existing saved config — let the backend read
			// the decrypted key from the database.
			testMutation.mutate({
				provider_config_id: providerConfig.id,
				...(trimmedBaseURL !== baseURL.trim() && {
					base_url: trimmedBaseURL,
				}),
			});
		} else {
			// Test with the API key the user typed in.
			testMutation.mutate({
				provider,
				api_key: effectiveApiKey,
				...(trimmedBaseURL && { base_url: trimmedBaseURL }),
			});
		}
	};

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
				const currentDisplayName =
					readOptionalString(providerConfig.display_name) ?? "";
				const currentBaseURL = baseURL.trim();
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

			setApiKeyTouched(false);
		} catch {
			// Error is surfaced via the mutation's error state
			// in ChatModelAdminPanel, no toast needed.
		}
	};

	const handleApiKeyFocus = () => {
		// Clear the placeholder on first focus so the user starts
		// with a blank field and Chrome doesn't try to autofill.
		if (!apiKeyTouched && apiKey === API_KEY_PLACEHOLDER) {
			setApiKey("");
			setApiKeyTouched(true);
		}
	};

	const isDisabled = providerConfigsUnavailable || isProviderMutationPending;

	return (
		<div className="flex min-h-full flex-col">
			{/* Back */}
			<button
				type="button"
				onClick={onBack}
				className="mb-4 inline-flex cursor-pointer items-center gap-0.5 bg-transparent border-0 p-0 text-sm text-content-secondary transition-colors hover:text-content-primary"
			>
				<ChevronLeftIcon className="h-4 w-4" />
				Back
			</button>

			{/* Provider header — editable name */}
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
								style={{ WebkitTextSecurity: "disc" } as React.CSSProperties}
								className="h-9 font-mono text-[13px]"
								placeholder="sk-..."
								value={apiKey}
								onFocus={handleApiKeyFocus}
								onChange={(e) => {
									setApiKey(e.target.value);
									setApiKeyTouched(true);
									testMutation.reset();
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
								onChange={(e) => {
									setBaseURLValue(e.target.value);
									testMutation.reset();
								}}
								disabled={isDisabled}
							/>
						</ProviderField>
					</div>

					{/* Test connection result */}
					{testMutation.isSuccess && (
						<div className="mt-4">
							{testMutation.data.model_list_not_supported ? (
								<div className="flex items-center gap-2 rounded-md border border-border bg-surface-primary px-3 py-2 text-sm text-content-secondary">
									<InfoIcon className="h-4 w-4 shrink-0" />
									Model listing is not supported for this provider. You can
									still configure models manually.
								</div>
							) : (
								<div className="flex items-center gap-2 rounded-md border border-border-success bg-surface-primary px-3 py-2 text-sm text-content-success">
									<CheckCircleIcon className="h-4 w-4 shrink-0" />
									Connected — {testMutation.data.models.length}{" "}
									{testMutation.data.models.length === 1 ? "model" : "models"}{" "}
									available
								</div>
							)}
						</div>
					)}
					{testMutation.isError && (
						<div className="mt-4">
							<div className="flex items-center gap-2 rounded-md border border-border-destructive bg-surface-primary px-3 py-2 text-sm text-content-destructive">
								<XCircleIcon className="h-4 w-4 shrink-0" />
								{testMutation.error instanceof Error
									? testMutation.error.message
									: "Failed to connect to provider"}
							</div>
						</div>
					)}

					{/* Footer — pushed to bottom */}
					<div className="mt-auto pt-6">
						<hr className="mb-4 border-0 border-t border-solid border-border" />
						{confirmingDelete && providerConfig ? (
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
											<Spinner className="h-4 w-4" loading />
										)}
										Delete provider
									</Button>
								</div>
							</div>
						) : (
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
									<Button
										variant="outline"
										size="lg"
										type="button"
										disabled={!canTest || testMutation.isPending}
										onClick={handleTest}
									>
										{testMutation.isPending && (
											<Spinner className="h-4 w-4" loading />
										)}
										Test connection
									</Button>
									<Button size="lg" type="submit" disabled={!canSave}>
										{isProviderMutationPending && (
											<Spinner className="h-4 w-4" loading />
										)}
										{providerConfig ? "Save changes" : "Create provider config"}
									</Button>
								</div>
							</div>
						)}
					</div>
				</form>
			)}
		</div>
	);
};

// ── Field wrapper ──────────────────────────────────────────────

interface ProviderFieldProps {
	label: string;
	htmlFor: string;
	required?: boolean;
	description?: string;
	children: React.ReactNode;
}

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
