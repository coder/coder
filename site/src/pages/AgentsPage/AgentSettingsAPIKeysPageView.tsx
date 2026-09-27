import { useFormik } from "formik";
import type { FC, FormEvent, ReactNode } from "react";
import { useEffect, useId, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	cancelUserBrowserGrant,
	cancelUserDeviceGrant,
	chatModelsKey,
	deleteUserChatProviderKey,
	exchangeUserBrowserGrant,
	initiateUserBrowserGrant,
	initiateUserDeviceGrant,
	upsertUserChatProviderKey,
	userAIDeviceGrant,
	userChatProviderConfigsKey,
} from "#/api/queries/chats";
import type {
	AIBrowserGrantExchangeResponse,
	AIBrowserGrantInitiateResponse,
	AIDeviceGrantInitiateResponse,
	ChatModel,
	UserChatProviderConfig,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { FormField } from "#/components/FormField/FormField";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Loader } from "#/components/Loader/Loader";
import { Spinner } from "#/components/Spinner/Spinner";
import { getFormHelpers } from "#/utils/formUtils";
import { SectionHeader } from "./components/SectionHeader";

const API_KEY_PLACEHOLDER = "••••••••••••••••";

type ProviderStatus = {
	label: string;
	variant: "default" | "green" | "warning";
	note?: string;
};

const providerKeyExpiryNote = (
	provider: UserChatProviderConfig,
): string | undefined => {
	if (!provider.oauth_expiry) {
		return undefined;
	}
	const expires = new Date(provider.oauth_expiry);
	if (Number.isNaN(expires.getTime())) {
		return undefined;
	}
	const when = expires.toLocaleString();
	if (provider.refresh_supported) {
		return `Access token expires ${when}. Coder refreshes it automatically.`;
	}
	return `Access token expires ${when}. Sign in again for a fresh code.`;
};

const getProviderStatus = (
	provider: UserChatProviderConfig,
): ProviderStatus => {
	if (!provider.byok_enabled) {
		return {
			label: "User keys disabled",
			variant: "default",
			note: "Personal API keys are disabled by your admin.",
		};
	}

	// Terminal refresh failure renders exactly one re-auth prompt per
	// provider (see the DeviceCodeSignIn banner below). The saved key is
	// kept: the user is blocked on this provider until re-auth, never
	// silently moved to another credential.
	if (provider.reauth_required) {
		return {
			label: "Sign-in expired",
			variant: "warning",
			note: "Your saved sign-in expired. Sign in again below. Your saved key is kept until the new sign-in lands.",
		};
	}

	if (provider.has_user_api_key) {
		return {
			label: "Key saved",
			variant: "green",
			note: providerKeyExpiryNote(provider),
		};
	}

	if (provider.has_central_api_key_fallback) {
		return {
			label: "Shared key",
			variant: "default",
			note: "The shared deployment key is being used. Add a personal key to use your own.",
		};
	}

	return {
		label: "No key",
		variant: "warning",
		note: "You must add a personal API key to use this provider.",
	};
};

type ProviderKeyPanelProps = {
	provider: UserChatProviderConfig;
	models: readonly ChatModel[];
	isModelsLoading: boolean;
	areModelsUnavailable: boolean;
};

/**
 * Paved device-code sign-in for one BYOK provider (ChatGPT first).
 * The user approves the displayed code at the provider, this panel polls
 * the grant, and the approved access token is saved through the same
 * user-keys mutation as a pasted key. The token itself is never shown.
 */
const DeviceCodeSignIn: FC<{ provider: UserChatProviderConfig }> = ({
	provider,
}) => {
	const queryClient = useQueryClient();
	const [grant, setGrant] = useState<AIDeviceGrantInitiateResponse | null>(
		null,
	);
	const [savedGrantId, setSavedGrantId] = useState<string | null>(null);

	const initiateMutation = useMutation(initiateUserDeviceGrant(queryClient));
	const cancelMutation = useMutation(cancelUserDeviceGrant());
	const saveMutation = useMutation(upsertUserChatProviderKey(queryClient));

	const pollQuery = useQuery({
		...userAIDeviceGrant(
			provider.provider_id,
			grant?.grant_id ?? "",
			Math.max((grant?.poll_interval ?? 5) * 1000, 1000),
		),
		enabled: grant !== null,
		retry: false,
	});

	const status = pollQuery.data?.status;
	const providerName = provider.display_name || provider.provider;

	// Save exactly once when the grant authorizes. Server-persisted grants
	// (no api_key in the poll response) already reached the key row: just
	// refresh the configs. Legacy grants still save through the existing
	// user-keys path so save/remove semantics stay identical to pasting.
	useEffect(() => {
		if (!grant || status !== "authorized") {
			return;
		}
		if (savedGrantId === grant.grant_id) {
			return;
		}
		const apiKey = pollQuery.data?.api_key;
		setSavedGrantId(grant.grant_id);
		void (async () => {
			try {
				if (!apiKey) {
					await Promise.all([
						queryClient.invalidateQueries({
							queryKey: userChatProviderConfigsKey,
						}),
						queryClient.invalidateQueries({ queryKey: chatModelsKey }),
					]);
					toast.success("Signed in. Personal key saved.");
					setGrant(null);
					return;
				}
				await saveMutation.mutateAsync({
					providerConfigId: provider.provider_id,
					req: { api_key: apiKey },
				});
				toast.success("Signed in. Personal key saved.");
				setGrant(null);
			} catch (error) {
				toast.error(getErrorMessage(error, "Error saving signed-in key."), {
					description: getErrorDetail(error),
				});
				setSavedGrantId(null);
			}
		})();
		// queryClient and saveMutation.mutateAsync are stable per query
		// client, so listing them keeps the effect exhaustive without
		// re-triggering the save.
	}, [
		grant,
		pollQuery.data,
		status,
		savedGrantId,
		provider.provider_id,
		queryClient,
		saveMutation.mutateAsync,
	]);

	const handleStart = async () => {
		try {
			const next = await initiateMutation.mutateAsync({
				providerConfigId: provider.provider_id,
			});
			setSavedGrantId(null);
			setGrant(next);
		} catch (error) {
			toast.error(getErrorMessage(error, "Error starting sign-in."), {
				description: getErrorDetail(error),
			});
		}
	};

	const handleCancel = async () => {
		if (grant) {
			try {
				await cancelMutation.mutateAsync({
					providerConfigId: provider.provider_id,
					grantId: grant.grant_id,
				});
			} catch {
				// Already terminal server-side; still reset the panel.
			}
		}
		setGrant(null);
	};

	if (!grant) {
		// Banner-dedupe: a terminal refresh failure renders exactly one
		// re-auth prompt per provider. The banner is keyed by the provider
		// panel, so a second identical failure re-renders this same banner
		// instead of stacking another prompt.
		if (provider.reauth_required) {
			return (
				<div className="mt-6 flex flex-col gap-2 border-t border-solid border-border pt-6">
					<div className="flex flex-col gap-3 sm:flex-row sm:items-center">
						<Button
							type="button"
							variant="outline"
							size="sm"
							onClick={handleStart}
							disabled={initiateMutation.isPending}
						>
							<Spinner loading={initiateMutation.isPending} />
							Sign in again with {providerName}
						</Button>
					</div>
					<p className="m-0 text-sm text-content-secondary">
						Your saved sign-in expired and automatic refresh stopped. Sign in
						again to continue. Your saved key is kept until the new sign-in
						lands.
					</p>
				</div>
			);
		}
		return (
			<div className="mt-6 flex flex-col gap-2 border-t border-solid border-border pt-6">
				<div className="flex flex-col gap-3 sm:flex-row sm:items-center">
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={handleStart}
						disabled={initiateMutation.isPending}
					>
						<Spinner loading={initiateMutation.isPending} />
						Sign in with {providerName}
					</Button>
				</div>
				<p className="m-0 text-sm text-content-secondary">
					Sign-in saves the provider credential as your personal key. Coder
					refreshes it automatically; if the sign-in expires, sign in again for
					a fresh code.
				</p>
			</div>
		);
	}

	const verificationUrl =
		grant.verification_uri_complete || grant.verification_uri;
	let statusLine: ReactNode;
	if (pollQuery.isError) {
		statusLine = (
			<p className="m-0 text-sm text-content-secondary">
				Could not reach the sign-in check. Keep this open and approve the code;
				the check retries automatically.
			</p>
		);
	} else {
		switch (status) {
			case "authorized":
				statusLine = (
					<p className="m-0 flex items-center gap-2 text-sm text-content-secondary">
						<Spinner loading size="sm" />
						{pollQuery.data?.api_key
							? "Approved. Saving your personal key."
							: "Approved. Personal key saved."}
					</p>
				);
				break;
			case "expired":
				statusLine = (
					<p className="m-0 text-sm text-content-secondary">
						{pollQuery.data?.reauth_message ??
							"This code expired. Start a fresh sign-in for a new code."}
					</p>
				);
				break;
			case "denied":
				statusLine = (
					<p className="m-0 text-sm text-content-secondary">
						Sign-in was denied at the provider. Start over to try again.
					</p>
				);
				break;
			case "canceled":
				statusLine = (
					<p className="m-0 text-sm text-content-secondary">
						Sign-in was canceled. Start over to try again.
					</p>
				);
				break;
			default:
				statusLine = (
					<p className="m-0 flex items-center gap-2 text-sm text-content-secondary">
						<Spinner loading size="sm" />
						Waiting for approval. This check repeats every{" "}
						{pollQuery.data?.poll_interval ?? grant.poll_interval} seconds until
						the code expires.
					</p>
				);
		}
	}

	const isTerminal =
		status === "expired" || status === "denied" || status === "canceled";

	return (
		<div className="mt-6 flex flex-col gap-3 border-t border-solid border-border pt-6">
			<p className="m-0 text-sm text-content-secondary">
				Open{" "}
				<a
					href={verificationUrl}
					target="_blank"
					rel="noreferrer"
					className="font-medium"
				>
					{grant.verification_uri}
				</a>{" "}
				and enter this code:
			</p>
			<p className="m-0 font-mono text-2xl font-semibold tracking-widest text-content-primary">
				{grant.user_code}
			</p>
			{statusLine}
			<div className="flex items-center gap-2">
				{isTerminal ? (
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={handleCancel}
					>
						Start over
					</Button>
				) : (
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={handleCancel}
						disabled={cancelMutation.isPending}
					>
						<Spinner loading={cancelMutation.isPending} />
						Cancel sign-in
					</Button>
				)}
			</div>
		</div>
	);
};

/**
 * Paved browser PKCE sign-in for one BYOK provider, next to the
 * device-code door. The server builds the provider authorize URL; the
 * user approves there, then pastes the localhost callback back here.
 * The server state-checks the paste and persists the credential itself,
 * so the exchange carries no key material back to the dashboard.
 */
const BrowserSignIn: FC<{ provider: UserChatProviderConfig }> = ({
	provider,
}) => {
	const queryClient = useQueryClient();
	const inputId = useId();
	const [grant, setGrant] = useState<AIBrowserGrantInitiateResponse | null>(
		null,
	);
	const [callbackInput, setCallbackInput] = useState("");
	const [exchangeResult, setExchangeResult] =
		useState<AIBrowserGrantExchangeResponse | null>(null);

	const initiateMutation = useMutation(initiateUserBrowserGrant(queryClient));
	const exchangeMutation = useMutation(exchangeUserBrowserGrant(queryClient));
	const cancelMutation = useMutation(cancelUserBrowserGrant());

	const providerName = provider.display_name || provider.provider;

	const handleStart = async () => {
		// Open the tab inside the click gesture so popup blockers let it
		// through, then navigate it once the authorize URL lands.
		const popup = window.open("about:blank", "_blank", "noopener,noreferrer");
		try {
			const next = await initiateMutation.mutateAsync({
				providerConfigId: provider.provider_id,
			});
			setExchangeResult(null);
			setCallbackInput("");
			setGrant(next);
			if (popup) {
				popup.location.href = next.authorize_url;
			}
		} catch (error) {
			popup?.close();
			toast.error(getErrorMessage(error, "Error starting browser sign-in."), {
				description: getErrorDetail(error),
			});
		}
	};

	const handleSubmit = async (event: FormEvent) => {
		event.preventDefault();
		if (
			!grant ||
			callbackInput.trim().length === 0 ||
			exchangeMutation.isPending
		) {
			return;
		}
		try {
			const result = await exchangeMutation.mutateAsync({
				providerConfigId: provider.provider_id,
				grantId: grant.grant_id,
				req: { input: callbackInput.trim() },
			});
			if (result.status === "authorized") {
				toast.success("Signed in. Personal key saved.");
				setGrant(null);
				setCallbackInput("");
				setExchangeResult(null);
				return;
			}
			setExchangeResult(result);
		} catch (error) {
			toast.error(getErrorMessage(error, "Error completing browser sign-in."), {
				description: getErrorDetail(error),
			});
		}
	};

	const handleCancel = async () => {
		if (grant) {
			try {
				await cancelMutation.mutateAsync({
					providerConfigId: provider.provider_id,
					grantId: grant.grant_id,
				});
			} catch {
				// Already terminal server-side; still reset the panel.
			}
		}
		setGrant(null);
		setCallbackInput("");
		setExchangeResult(null);
	};

	if (!grant) {
		// The device-code banner carries the one re-auth prompt, so this
		// door offers only its button here instead of a second prompt.
		if (provider.reauth_required) {
			return (
				<div className="mt-6 flex flex-col gap-2 border-t border-solid border-border pt-6">
					<div className="flex flex-col gap-3 sm:flex-row sm:items-center">
						<Button
							type="button"
							variant="outline"
							size="sm"
							onClick={handleStart}
							disabled={initiateMutation.isPending}
						>
							<Spinner loading={initiateMutation.isPending} />
							Sign in again with {providerName} in your browser
						</Button>
					</div>
				</div>
			);
		}
		return (
			<div className="mt-6 flex flex-col gap-2 border-t border-solid border-border pt-6">
				<div className="flex flex-col gap-3 sm:flex-row sm:items-center">
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={handleStart}
						disabled={initiateMutation.isPending}
					>
						<Spinner loading={initiateMutation.isPending} />
						Sign in with {providerName} in your browser
					</Button>
				</div>
				<p className="m-0 text-sm text-content-secondary">
					Approve the sign-in in your browser, then paste the callback back
					here. Coder refreshes it automatically; if the sign-in expires, sign
					in again.
				</p>
			</div>
		);
	}

	const status = exchangeResult?.status;
	const isTerminal = status === "expired" || status === "canceled";
	let statusLine: ReactNode;
	if (status === "expired") {
		statusLine = (
			<p className="m-0 text-sm text-content-secondary">
				{exchangeResult?.reauth_message ??
					"This sign-in expired. Start over for a fresh one."}
			</p>
		);
	} else if (status === "canceled") {
		statusLine = (
			<p className="m-0 text-sm text-content-secondary">
				Sign-in was canceled. Start over to try again.
			</p>
		);
	} else {
		statusLine = (
			<p className="m-0 flex items-center gap-2 text-sm text-content-secondary">
				<Spinner loading={exchangeMutation.isPending} size="sm" />
				Waiting for the callback paste. The sign-in stays open until it expires.
			</p>
		);
	}

	return (
		<div className="mt-6 flex flex-col gap-3 border-t border-solid border-border pt-6">
			<p className="m-0 text-sm text-content-secondary">
				Open{" "}
				<a
					href={grant.authorize_url}
					target="_blank"
					rel="noreferrer"
					className="font-medium"
				>
					{providerName} sign-in
				</a>{" "}
				in your browser, approve it, then paste the localhost callback URL or
				code below.
			</p>
			<form className="flex flex-col gap-3" onSubmit={handleSubmit}>
				<div className="flex flex-col gap-2">
					<Label htmlFor={inputId}>Authorization callback</Label>
					<Input
						id={inputId}
						type="text"
						value={callbackInput}
						onChange={(event) => setCallbackInput(event.target.value)}
						placeholder="http://localhost:1455/auth/callback?code=..."
						disabled={exchangeMutation.isPending}
						className="h-8 font-mono"
						spellCheck={false}
						autoComplete="off"
					/>
				</div>
				<div className="flex items-center gap-2">
					{isTerminal ? (
						<Button
							type="button"
							variant="outline"
							size="sm"
							onClick={handleCancel}
						>
							Start over
						</Button>
					) : (
						<>
							<Button
								type="submit"
								size="sm"
								disabled={
									callbackInput.trim().length === 0 ||
									exchangeMutation.isPending
								}
							>
								<Spinner loading={exchangeMutation.isPending} />
								Complete sign-in
							</Button>
							<Button
								type="button"
								variant="outline"
								size="sm"
								onClick={handleCancel}
								disabled={
									cancelMutation.isPending || exchangeMutation.isPending
								}
							>
								<Spinner loading={cancelMutation.isPending} />
								Cancel sign-in
							</Button>
						</>
					)}
				</div>
			</form>
			{statusLine}
		</div>
	);
};

const ProviderKeyPanel: FC<ProviderKeyPanelProps> = ({
	provider,
	models,
	isModelsLoading,
	areModelsUnavailable,
}) => {
	const queryClient = useQueryClient();
	const headingId = useId();

	const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);

	const saveMutation = useMutation(upsertUserChatProviderKey(queryClient));
	const removeMutation = useMutation(deleteUserChatProviderKey(queryClient));
	const isBusy = saveMutation.isPending || removeMutation.isPending;

	const form = useFormik({
		initialValues: {
			apiKey: provider.has_user_api_key ? API_KEY_PLACEHOLDER : "",
		},
		onSubmit: async (values, helpers) => {
			const apiKey = values.apiKey.trim();
			if (
				!provider.byok_enabled ||
				apiKey.length === 0 ||
				apiKey === API_KEY_PLACEHOLDER ||
				isBusy
			) {
				return;
			}

			try {
				await saveMutation.mutateAsync({
					providerConfigId: provider.provider_id,
					req: { api_key: apiKey },
				});
				helpers.resetForm({ values: { apiKey: API_KEY_PLACEHOLDER } });
				toast.success("API key saved.");
			} catch (error) {
				toast.error(getErrorMessage(error, "Error saving API key."), {
					description: getErrorDetail(error),
				});
			}
		},
	});
	const getFieldHelpers = getFormHelpers(form);

	const status = getProviderStatus(provider);
	const enabledModels = models.filter(
		(model) => model.enabled && model.ai_provider_id === provider.provider_id,
	);
	const saveDisabled =
		!provider.byok_enabled ||
		form.values.apiKey.trim().length === 0 ||
		form.values.apiKey === API_KEY_PLACEHOLDER ||
		isBusy;
	const inputDisabled = !provider.byok_enabled || isBusy;
	const providerName = provider.display_name || provider.provider;

	const handleApiKeyFocus = () => {
		if (form.values.apiKey === API_KEY_PLACEHOLDER) {
			void form.setFieldValue("apiKey", "");
		}
	};

	const handleRemoveKey = async () => {
		try {
			await removeMutation.mutateAsync(provider.provider_id);
			setIsDeleteDialogOpen(false);
			form.resetForm({ values: { apiKey: "" } });
			toast.success("API key removed.");
		} catch (error) {
			toast.error(getErrorMessage(error, "Error removing API key."), {
				description: getErrorDetail(error),
			});
		}
	};

	const deleteDescription = provider.has_central_api_key_fallback
		? "Requests will fall back to the shared deployment key for this provider."
		: "You will need to add a new key before you can use this provider again.";

	let enabledModelsContent: ReactNode;
	if (isModelsLoading) {
		enabledModelsContent = <Spinner size="sm" loading label="Loading models" />;
	} else if (enabledModels.length > 0) {
		enabledModelsContent = (
			<div className="flex flex-wrap gap-2">
				{enabledModels.map((model) => (
					<Badge key={model.id} size="md" variant="default">
						{model.display_name || model.model}
					</Badge>
				))}
			</div>
		);
	} else if (areModelsUnavailable) {
		enabledModelsContent = (
			<p className="m-0 text-sm text-content-secondary">
				Enabled models are temporarily unavailable.
			</p>
		);
	} else {
		enabledModelsContent = (
			<p className="m-0 text-sm text-content-secondary">
				No enabled models configured.
			</p>
		);
	}

	return (
		<article
			className="rounded-lg border border-solid border-border p-6"
			aria-labelledby={headingId}
		>
			<div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
				<div className="space-y-2">
					<h5
						id={headingId}
						className="m-0 text-lg font-medium text-content-primary"
					>
						{providerName}
					</h5>
					{status.note && (
						<p className="m-0 text-sm text-content-secondary">{status.note}</p>
					)}
				</div>
				<Badge size="md" variant={status.variant} className="w-fit">
					{status.label}
				</Badge>
			</div>

			<form className="mt-6" onSubmit={form.handleSubmit}>
				<div className="flex flex-col gap-3 lg:flex-row lg:items-end">
					<div className="min-w-0 lg:flex-1">
						<FormField
							field={getFieldHelpers("apiKey")}
							label="API Key"
							type="password"
							placeholder="sk-..."
							disabled={inputDisabled}
							className="h-8 font-mono"
							ignorePasswordManagers
							onFocus={handleApiKeyFocus}
						/>
					</div>
					<div className="flex items-center gap-2">
						<Button type="submit" size="sm" disabled={saveDisabled}>
							<Spinner loading={saveMutation.isPending} />
							Save
						</Button>
						{provider.has_user_api_key && (
							<Button
								type="button"
								variant="outline"
								size="sm"
								onClick={() => setIsDeleteDialogOpen(true)}
								disabled={isBusy}
							>
								Remove
							</Button>
						)}
					</div>
				</div>
			</form>

			{provider.device_flow_supported && provider.byok_enabled && (
				<DeviceCodeSignIn provider={provider} />
			)}

			{provider.browser_flow_supported && provider.byok_enabled && (
				<BrowserSignIn provider={provider} />
			)}

			<div className="mt-6 flex flex-col gap-2">
				<p className="m-0 text-sm font-medium text-content-primary">
					Enabled models
				</p>
				{areModelsUnavailable && enabledModels.length > 0 && (
					<p className="m-0 text-sm text-content-secondary">
						Some enabled models are temporarily unavailable.
					</p>
				)}
				{enabledModelsContent}
			</div>

			<ConfirmDialog
				open={isDeleteDialogOpen}
				onClose={() => setIsDeleteDialogOpen(false)}
				onConfirm={handleRemoveKey}
				title="Remove API key"
				description={deleteDescription}
				confirmText="Remove"
				confirmLoading={removeMutation.isPending}
				type="delete"
			/>
		</article>
	);
};

export type AgentSettingsAPIKeysPageViewProps = {
	error: unknown;
	isLoading: boolean;
	providers: readonly UserChatProviderConfig[];
	models: readonly ChatModel[];
	isModelsLoading: boolean;
	areModelsUnavailable: boolean;
};

export const AgentSettingsAPIKeysPageView: FC<
	AgentSettingsAPIKeysPageViewProps
> = ({
	error,
	isLoading,
	providers,
	models,
	isModelsLoading,
	areModelsUnavailable,
}) => {
	return (
		<section className="flex flex-col gap-8">
			<SectionHeader
				label="Secrets (API keys)"
				description="Add a personal API key for each provider. Your personal key takes precedence over the shared deployment key when both are available."
			/>
			{error ? (
				<ErrorAlert error={error} />
			) : isLoading ? (
				<Loader />
			) : providers.length === 0 ? (
				<EmptyState
					message="No providers allow personal API keys."
					description="Ask your administrator to enable personal API keys for at least one provider."
				/>
			) : (
				<div className="flex flex-col gap-4">
					{providers.map((provider) => (
						<ProviderKeyPanel
							key={provider.provider_id}
							provider={provider}
							models={models}
							isModelsLoading={isModelsLoading}
							areModelsUnavailable={areModelsUnavailable}
						/>
					))}
				</div>
			)}
		</section>
	);
};
