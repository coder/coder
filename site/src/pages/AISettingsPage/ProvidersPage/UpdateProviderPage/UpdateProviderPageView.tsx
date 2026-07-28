import { isAxiosError } from "axios";
import { ArrowLeftIcon } from "lucide-react";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link, Navigate, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import {
	aiProvider,
	aiProviderKeyFor,
	deleteAIProviderMutation,
	updateAIProviderMutation,
} from "#/api/queries/aiProviders";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { DeleteDialog } from "#/components/Dialogs/DeleteDialog/DeleteDialog";
import { Loader } from "#/components/Loader/Loader";
import { SettingsHeaderTitle } from "#/components/SettingsHeader/SettingsHeader";
import { Switch } from "#/components/Switch/Switch";
import { pageTitle } from "#/utils/page";
import { ProviderForm } from "../components/ProviderForm";
import { getProviderIcon } from "../components/ProviderIcon";
import {
	aiProviderToFormValues,
	bedrockExternalId,
	getProviderDisplayType,
	hasBedrockStoredCredentials,
	isBedrockProvider,
	providerFormValuesToUpdate,
} from "../components/providerFormApiMap";

const BACK_HREF = "/ai/settings/providers";

const UpdateProviderPageView: React.FC = () => {
	const { providerId } = useParams<{ providerId: string }>();
	const queryClient = useQueryClient();
	const navigate = useNavigate();

	const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

	const providerQuery = useQuery({
		...aiProvider(providerId ?? ""),
		enabled: Boolean(providerId),
	});

	const provider = providerQuery.data;
	// Copilot has no stored credential, and Bedrock keeps its secrets in
	// settings, so only the remaining types surface the api_keys UI.
	const providerUsesApiKeys =
		provider !== undefined &&
		!isBedrockProvider(provider) &&
		provider.type !== "copilot";

	const updateMutation = useMutation(
		updateAIProviderMutation(queryClient, providerId ?? ""),
	);

	const deleteMutation = useMutation(
		deleteAIProviderMutation(queryClient, providerId ?? ""),
	);

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
		providerUsesApiKeys && provider.api_keys.length > 0;
	const openAiAnthropicMaskedApiKey = providerUsesApiKeys
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
						src={
							provider.icon || getProviderIcon(getProviderDisplayType(provider))
						}
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
							href="/ai/settings/models"
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
											queryClient.setQueryData(
												aiProviderKeyFor(providerId),
												updated,
											);
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
						bedrockExternalId={bedrockExternalId(provider)}
						openAiAnthropicSavedApiKey={openAiAnthropicSavedApiKey}
						openAiAnthropicMaskedApiKey={openAiAnthropicMaskedApiKey}
						initialValues={aiProviderToFormValues(provider)}
						isLoading={updateMutation.isPending}
						submitError={updateMutation.error}
						onSubmit={async (values) => {
							const request = providerFormValuesToUpdate(values, provider);
							try {
								const updated = await updateMutation.mutateAsync(request);
								queryClient.setQueryData(aiProviderKeyFor(providerId), updated);
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
				<DeleteDialog
					key={provider.name}
					isOpen={deleteDialogOpen}
					title="Delete provider"
					entity="provider"
					name={provider.name}
					confirmLoading={deleteMutation.isPending}
					onCancel={() => {
						setDeleteDialogOpen(false);
					}}
					onConfirm={() => {
						deleteMutation.mutate(undefined, {
							onSuccess: () => {
								toast.success(
									`Provider "${provider.display_name || provider.name}" deleted.`,
								);
								setDeleteDialogOpen(false);
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
						});
					}}
				/>
			</div>
		</>
	);
};

export default UpdateProviderPageView;
