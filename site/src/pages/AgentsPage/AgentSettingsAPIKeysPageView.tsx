import { useFormik } from "formik";
import type { FC, ReactNode } from "react";
import { useId, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	deleteUserChatProviderKey,
	upsertUserChatProviderKey,
} from "#/api/queries/chats";
import type { ChatModel, UserChatProviderConfig } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { FormField } from "#/components/FormField/FormField";
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

	if (provider.has_user_api_key) {
		return {
			label: "Key saved",
			variant: "green",
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

interface ProviderKeyPanelProps {
	provider: UserChatProviderConfig;
	models: readonly ChatModel[];
	isModelsLoading: boolean;
	areModelsUnavailable: boolean;
}

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

export interface AgentSettingsAPIKeysPageViewProps {
	error: unknown;
	isLoading: boolean;
	providers: readonly UserChatProviderConfig[];
	models: readonly ChatModel[];
	isModelsLoading: boolean;
	areModelsUnavailable: boolean;
}

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
