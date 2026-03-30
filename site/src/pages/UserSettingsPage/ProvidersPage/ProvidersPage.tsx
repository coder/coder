import { type FC, useEffect, useId, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	chatModelConfigs,
	deleteUserChatProviderKey,
	upsertUserChatProviderKey,
	userChatProviderConfigs,
} from "#/api/queries/chats";
import type {
	ChatModelConfig,
	UserChatProviderConfig,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Input } from "#/components/Input/Input";
import { Loader } from "#/components/Loader/Loader";
import { Section } from "../Section";

const API_KEY_PLACEHOLDER = "••••••••••••••••";

type ProviderStatus = {
	label: string;
	variant: "default" | "green" | "warning";
	note?: string;
};

const getProviderStatus = (
	provider: UserChatProviderConfig,
): ProviderStatus => {
	if (provider.has_user_api_key) {
		return {
			label: "Key saved",
			variant: "green",
		};
	}

	if (provider.has_central_api_key_fallback) {
		return {
			label: "Using shared key",
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

interface ProviderKeyPanelProps {
	provider: UserChatProviderConfig;
	models: ChatModelConfig[];
	isModelsLoading: boolean;
	areModelsUnavailable: boolean;
	isSaving: boolean;
	isRemoving: boolean;
	onSave: (
		providerConfigId: string,
		apiKey: string,
		onSuccess: () => void,
	) => void;
	onRemove: (providerConfigId: string, onSuccess: () => void) => void;
}

const ProviderKeyPanel: FC<ProviderKeyPanelProps> = ({
	provider,
	models,
	isModelsLoading,
	areModelsUnavailable,
	isSaving,
	isRemoving,
	onSave,
	onRemove,
}) => {
	const apiKeyInputId = useId();
	const [apiKey, setApiKey] = useState(
		provider.has_user_api_key ? API_KEY_PLACEHOLDER : "",
	);
	const [apiKeyTouched, setApiKeyTouched] = useState(false);
	const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);

	useEffect(() => {
		setApiKey(provider.has_user_api_key ? API_KEY_PLACEHOLDER : "");
		setApiKeyTouched(false);
	}, [provider.has_user_api_key]);

	const status = getProviderStatus(provider);
	const enabledModels = models.filter((model) => {
		return model.enabled && model.provider === provider.provider;
	});
	const trimmedApiKey = apiKey.trim();
	const saveDisabled =
		trimmedApiKey.length === 0 ||
		apiKey === API_KEY_PLACEHOLDER ||
		isSaving ||
		isRemoving;
	const inputDisabled = isSaving || isRemoving;
	const providerName = provider.display_name || provider.provider;

	const handleApiKeyFocus = () => {
		if (!apiKeyTouched && apiKey === API_KEY_PLACEHOLDER) {
			setApiKey("");
			setApiKeyTouched(true);
		}
	};

	const handleSave = () => {
		if (saveDisabled) {
			return;
		}

		onSave(provider.provider_id, trimmedApiKey, () => {
			setApiKey(API_KEY_PLACEHOLDER);
			setApiKeyTouched(false);
		});
	};

	const handleRemoveKey = () => {
		onRemove(provider.provider_id, () => {
			setApiKey("");
			setApiKeyTouched(false);
			setIsDeleteDialogOpen(false);
		});
	};

	const deleteDescription = provider.has_central_api_key_fallback
		? "This will remove your personal API key. Requests will fall back to the shared deployment key for this provider."
		: "This will remove your personal API key. You will need to add a new key before you can use this provider again.";

	return (
		<article className="rounded-lg border border-solid border-border p-6">
			<div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
				<div className="space-y-2">
					<h5 className="m-0 text-lg font-medium text-content-primary">
						{providerName}
					</h5>
					{status.note && (
						<p className="m-0 text-sm text-content-secondary">{status.note}</p>
					)}
				</div>
				<Badge size="sm" variant={status.variant} className="w-fit">
					{status.label}
				</Badge>
			</div>

			<div className="mt-6 flex flex-col gap-3">
				<label
					htmlFor={apiKeyInputId}
					className="text-sm font-medium text-content-primary"
				>
					API Key
				</label>
				<div className="flex flex-col gap-3 lg:flex-row lg:items-start">
					<Input
						id={apiKeyInputId}
						name={`provider-api-key-${provider.provider_id}`}
						type="password"
						autoComplete="off"
						data-1p-ignore
						data-lpignore="true"
						data-form-type="other"
						data-bwignore
						className="h-9 font-mono text-[13px] lg:flex-1"
						placeholder="sk-..."
						value={apiKey}
						onFocus={handleApiKeyFocus}
						onChange={(event) => {
							setApiKey(event.target.value);
							setApiKeyTouched(true);
						}}
						disabled={inputDisabled}
					/>
					<div className="flex items-center gap-2">
						<Button size="sm" onClick={handleSave} disabled={saveDisabled}>
							Save
						</Button>
						{provider.has_user_api_key && (
							<Button
								variant="outline"
								size="sm"
								onClick={() => setIsDeleteDialogOpen(true)}
								disabled={inputDisabled}
							>
								Remove
							</Button>
						)}
					</div>
				</div>
			</div>

			<div className="mt-6 flex flex-col gap-2">
				<p className="m-0 text-sm font-medium text-content-primary">
					Enabled models
				</p>
				{areModelsUnavailable ? (
					<p className="m-0 text-sm text-content-secondary">
						Enabled model badges are temporarily unavailable.
					</p>
				) : isModelsLoading ? (
					<p className="m-0 text-sm text-content-secondary">
						Loading models...
					</p>
				) : enabledModels.length > 0 ? (
					<div className="flex flex-wrap gap-2">
						{enabledModels.map((model) => (
							<Badge key={model.id} size="xs" variant="default">
								{model.display_name || model.model}
							</Badge>
						))}
					</div>
				) : (
					<p className="m-0 text-sm text-content-secondary">
						No enabled models configured.
					</p>
				)}
			</div>

			<ConfirmDialog
				open={isDeleteDialogOpen}
				onClose={() => setIsDeleteDialogOpen(false)}
				onConfirm={handleRemoveKey}
				title="Remove API key?"
				description={deleteDescription}
				confirmText="Remove"
				confirmLoading={isRemoving}
				type="delete"
			/>
		</article>
	);
};

const ProvidersPage: FC = () => {
	const queryClient = useQueryClient();
	const {
		data: providers,
		error: providersError,
		isLoading: providersLoading,
	} = useQuery(userChatProviderConfigs());
	const {
		data: models,
		error: modelsError,
		isLoading: modelsLoading,
	} = useQuery(chatModelConfigs());
	const upsertMutation = useMutation(upsertUserChatProviderKey(queryClient));
	const deleteMutation = useMutation(deleteUserChatProviderKey(queryClient));

	const error = providersError;
	const isLoading = providersLoading;
	const areModelsUnavailable = Boolean(modelsError);
	const providerConfigs = providers ?? [];
	const modelConfigs = models ?? [];

	return (
		<Section
			layout="fluid"
			title="Provider Settings"
			description="Add a personal API key for each provider. Your personal key takes precedence over the shared deployment key when both are available."
		>
			{error ? (
				<ErrorAlert error={error} />
			) : isLoading ? (
				<Loader />
			) : providerConfigs.length === 0 ? (
				<EmptyState
					message="No providers are configured."
					description="Ask your administrator to enable at least one provider before adding a personal API key."
				/>
			) : (
				<div className="flex flex-col gap-4">
					{providerConfigs.map((provider) => (
						<ProviderKeyPanel
							key={provider.provider_id}
							provider={provider}
							models={modelConfigs}
							isModelsLoading={modelsLoading}
							areModelsUnavailable={areModelsUnavailable}
							isSaving={upsertMutation.isPending}
							isRemoving={deleteMutation.isPending}
							onSave={(providerConfigId, apiKey, onSuccess) => {
								upsertMutation.mutate(
									{
										providerConfigId,
										req: { api_key: apiKey },
									},
									{
										onSuccess: () => {
											onSuccess();
											toast.success("API key saved.");
										},
										onError: (mutationError) => {
											toast.error(
												getErrorMessage(mutationError, "Error saving API key."),
												{
													description: getErrorDetail(mutationError),
												},
											);
										},
									},
								);
							}}
							onRemove={(providerConfigId, onSuccess) => {
								deleteMutation.mutate(providerConfigId, {
									onSuccess: () => {
										onSuccess();
										toast.success("API key removed.");
									},
									onError: (mutationError) => {
										toast.error(
											getErrorMessage(mutationError, "Error removing API key."),
											{
												description: getErrorDetail(mutationError),
											},
										);
									},
								});
							}}
						/>
					))}
				</div>
			)}
		</Section>
	);
};

export default ProvidersPage;
