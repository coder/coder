import { isAxiosError } from "axios";
import { ArrowLeftIcon } from "lucide-react";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link, Navigate, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { API } from "#/api/api";
import { getErrorMessage } from "#/api/errors";
import {
	aiProvider,
	deleteAIProviderMutation,
	updateAIProviderMutation,
} from "#/api/queries/aiProviders";
import {
	chatModelConfigs,
	invalidateChatConfigurationQueries,
} from "#/api/queries/chats";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Input } from "#/components/Input/Input";
import { Loader } from "#/components/Loader/Loader";
import { SettingsHeaderTitle } from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import { cascadeDisableProviderModels } from "#/pages/AISettingsPage/utils/providerDelete";
import { pageTitle } from "#/utils/page";
import { ProviderForm } from "../components/ProviderForm";
import { getProviderIcon } from "../components/ProviderIcon";
import {
	aiProviderToFormValues,
	getProviderDisplayType,
	hasBedrockStoredCredentials,
	isBedrockProvider,
	providerFormValuesToUpdate,
} from "../components/providerFormApiMap";

const BACK_HREF = "/ai/settings";

const UpdateProviderPageView: React.FC = () => {
	const { providerId } = useParams<{ providerId: string }>();
	const queryClient = useQueryClient();
	const navigate = useNavigate();

	const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
	const [deleteConfirmText, setDeleteConfirmText] = useState("");
	const [isCascadeDeleting, setIsCascadeDeleting] = useState(false);

	const providerQuery = useQuery({
		...aiProvider(providerId ?? ""),
		enabled: Boolean(providerId),
	});

	const provider = providerQuery.data;
	const providerIsOpenAiAnthropic =
		provider !== undefined && !isBedrockProvider(provider);

	const updateMutation = useMutation(
		updateAIProviderMutation(queryClient, providerId ?? ""),
	);

	const deleteMutation = useMutation(
		deleteAIProviderMutation(queryClient, providerId ?? ""),
	);

	const modelConfigsQuery = useQuery({
		...chatModelConfigs(),
		enabled: Boolean(providerId),
	});

	const associatedModels = (modelConfigsQuery.data ?? []).filter(
		(mc) => mc.ai_provider_id === provider?.id,
	);
	const associatedModelCount = associatedModels.length;

	// Rendered into every non-redirect return so the document title reflects
	// the provider as soon as we know it; falls back to a placeholder while
	// the query is in flight.
	const title = (
		<title>
			{pageTitle(
				(provider?.display_name || provider?.name) ?? "Loading...",
				"AI Providers",
			)}
		</title>
	);

	if (!providerId) {
		return <Navigate to={BACK_HREF} replace />;
	}

	if (providerQuery.isLoading) {
		return (
			<>
				{title}
				<Loader fullscreen />
			</>
		);
	}

	if (providerQuery.isError) {
		const status = isAxiosError(providerQuery.error)
			? providerQuery.error.response?.status
			: undefined;
		if (status === 404) {
			return <Navigate to={BACK_HREF} replace />;
		}
		return (
			<>
				{title}
				<div className="flex flex-col gap-4">
					<p className="text-content-secondary">
						{getErrorMessage(providerQuery.error, "Failed to load provider.")}
					</p>
					<Link to={BACK_HREF} className="-ml-3">
						<Button variant="subtle">
							<ArrowLeftIcon />
							<span>Back to providers</span>
						</Button>
					</Link>
				</div>
			</>
		);
	}

	if (!provider) {
		return <Navigate to={BACK_HREF} replace />;
	}

	const openAiAnthropicSavedApiKey =
		providerIsOpenAiAnthropic && provider.api_keys.length > 0;
	const openAiAnthropicMaskedApiKey = providerIsOpenAiAnthropic
		? provider.api_keys[0]?.masked
		: undefined;

	return (
		<>
			{title}
			<div className="flex justify-between items-center">
				<Link to={BACK_HREF} className="-ml-3">
					<Button variant="subtle">
						<ArrowLeftIcon />
						<span>Back to providers</span>
					</Button>
				</Link>
				<Button
					type="button"
					variant="destructive"
					disabled={updateMutation.isPending || deleteMutation.isPending}
					onClick={() => {
						setDeleteDialogOpen(true);
					}}
				>
					<span>Delete</span>
				</Button>
			</div>
			<div className="flex flex-col gap-6 pt-6">
				<div className="flex items-center gap-4 min-w-0">
					<Avatar
						variant="icon"
						size="lg"
						src={getProviderIcon(getProviderDisplayType(provider))}
					/>
					<SettingsHeaderTitle>
						<span className="block min-w-0 truncate">
							{provider.display_name || provider.name}
						</span>
					</SettingsHeaderTitle>
					{!provider.enabled && <Badge variant="default">Disabled</Badge>}
				</div>
				<div className="flex items-center justify-between w-full">
					<p className="text-sm text-content-secondary m-0">
						Add or update models for this provider.{" "}
						<a
							href="/agents/settings/models"
							className="text-content-link no-underline hover:underline"
						>
							Model settings
						</a>
					</p>
					<div className="flex items-center gap-2">
						<Switch
							checked={provider.enabled}
							onCheckedChange={(checked) => {
								updateMutation.mutate(
									{ enabled: checked },
									{
										onSuccess: (updated) => {
											toast.success(
												`Provider "${updated.display_name || updated.name}" ${checked ? "enabled" : "disabled"}.`,
											);
										},
									},
								);
							}}
							disabled={updateMutation.isPending}
							aria-label="Provider enabled"
						/>
						<span className="text-sm">Enable</span>
					</div>
				</div>
				<div className="border border-solid p-6 rounded-lg">
					<ProviderForm
						editing
						key={provider.id}
						bedrockSavedAccessCredentials={hasBedrockStoredCredentials(
							provider,
						)}
						openAiAnthropicSavedApiKey={openAiAnthropicSavedApiKey}
						openAiAnthropicMaskedApiKey={openAiAnthropicMaskedApiKey}
						initialValues={aiProviderToFormValues(provider)}
						isLoading={updateMutation.isPending}
						submitError={updateMutation.error}
						onSubmit={async (values) => {
							const request = providerFormValuesToUpdate(values, provider);
							try {
								const updated = await updateMutation.mutateAsync(request);
								toast.success(
									`Provider "${updated.display_name || updated.name}" updated.`,
								);
							} catch (error) {
								toast.error(
									getErrorMessage(
										error,
										`Failed to update provider "${provider.display_name || provider.name}".`,
									),
								);
							}
						}}
					/>
				</div>
				<Dialog
					open={deleteDialogOpen}
					onOpenChange={(open) => {
						if (!open && !isCascadeDeleting && !deleteMutation.isPending) {
							setDeleteDialogOpen(false);
							setDeleteConfirmText("");
						}
					}}
				>
					<DialogContent variant="destructive">
						<DialogHeader>
							<DialogTitle>Delete provider</DialogTitle>
							<DialogDescription>
								Deleting this provider is irreversible!
							</DialogDescription>
						</DialogHeader>
						<div className="flex flex-col gap-3 text-sm text-content-secondary">
							{associatedModelCount > 0 && (
								<ul className="m-0 pl-5">
									<li>
										Deleting this provider will also disable{" "}
										<strong className="text-content-primary">
											{associatedModelCount}{" "}
											{associatedModelCount === 1 ? "model" : "models"}
										</strong>{" "}
										from your settings.
									</li>
								</ul>
							)}
							<p className="m-0">
								Type{" "}
								<strong className="text-content-primary">
									{provider.name}
								</strong>{" "}
								to confirm.
							</p>
							<Input
								id="delete-confirm"
								aria-label={`Type ${provider.name} to confirm`}
								autoFocus
								autoComplete="off"
								placeholder={provider.name}
								value={deleteConfirmText}
								onChange={(e) => setDeleteConfirmText(e.target.value)}
							/>
						</div>
						<DialogFooter>
							<Button
								variant="outline"
								onClick={() => {
									setDeleteDialogOpen(false);
									setDeleteConfirmText("");
								}}
								disabled={isCascadeDeleting || deleteMutation.isPending}
							>
								Cancel
							</Button>
							<Button
								variant="destructive"
								className="disabled:border-border"
								disabled={
									deleteConfirmText !== provider.name ||
									isCascadeDeleting ||
									deleteMutation.isPending ||
									modelConfigsQuery.isLoading ||
									modelConfigsQuery.isError
								}
								onClick={() => {
									const deleteAll = async () => {
										setIsCascadeDeleting(true);
										try {
											await cascadeDisableProviderModels({
												associatedModels,
												allModels: modelConfigsQuery.data ?? [],
												updateModelConfig:
													API.experimental.updateChatModelConfig,
											});
											await invalidateChatConfigurationQueries(queryClient);
											deleteMutation.mutate(undefined, {
												onSuccess: () => {
													toast.success(
														`Provider "${provider.display_name || provider.name}" deleted.`,
													);
													setDeleteDialogOpen(false);
													setDeleteConfirmText("");
													void navigate(BACK_HREF, { replace: true });
												},
												onError: (error) => {
													toast.error(
														getErrorMessage(
															error,
															`Failed to delete provider "${provider.display_name || provider.name}".`,
														),
													);
												},
												onSettled: () => {
													setIsCascadeDeleting(false);
												},
											});
										} catch (error) {
											toast.error(
												getErrorMessage(error, "Failed to delete provider."),
											);
											setIsCascadeDeleting(false);
											await invalidateChatConfigurationQueries(queryClient);
										}
									};
									void deleteAll();
								}}
							>
								{(isCascadeDeleting || deleteMutation.isPending) && (
									<Spinner className="h-4 w-4" loading />
								)}
								Delete provider
							</Button>
						</DialogFooter>
					</DialogContent>
				</Dialog>
			</div>
		</>
	);
};

export default UpdateProviderPageView;
