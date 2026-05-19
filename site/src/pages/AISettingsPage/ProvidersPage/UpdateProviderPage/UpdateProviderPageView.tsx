import { isAxiosError } from "axios";
import { ArrowLeftIcon, TrashIcon } from "lucide-react";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Link, Navigate, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { getErrorMessage } from "#/api/errors";
import {
	aiProvider,
	deleteAIProviderMutation,
	updateAIProviderMutation,
} from "#/api/queries/aiProviders";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import { DeleteDialog } from "#/components/Dialogs/DeleteDialog/DeleteDialog";
import { Loader } from "#/components/Loader/Loader";
import {
	PageHeader,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import { pageTitle } from "#/utils/page";
import { ProviderForm } from "../components/ProviderForm";
import { getProviderIcon } from "../components/ProviderIcon";
import {
	aiProviderToFormValues,
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
				<div className="pt-4 px-6 flex flex-col gap-4">
					<p className="text-content-secondary">
						{getErrorMessage(providerQuery.error, "Failed to load provider.")}
					</p>
					<Link to={BACK_HREF}>
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
			<div className="pt-4 px-6">
				<Link to={BACK_HREF}>
					<Button variant="subtle">
						<ArrowLeftIcon />
						<span>Back to providers</span>
					</Button>
				</Link>
			</div>
			<div className="mx-auto w-full max-w-screen-sm flex flex-col gap-6">
				<PageHeader
					className="pt-6 pb-0"
					actions={
						<Button
							type="button"
							variant="destructive"
							disabled={updateMutation.isPending || deleteMutation.isPending}
							onClick={() => {
								setDeleteDialogOpen(true);
							}}
						>
							<TrashIcon />
							<span>Delete provider</span>
						</Button>
					}
				>
					<div className="flex items-center gap-4 min-w-0">
						<Avatar
							variant="icon"
							size="lg"
							src={getProviderIcon(provider.type)}
						/>
						<PageHeaderTitle
							className="min-w-0"
							title={provider.display_name || provider.name}
						>
							{/* PageHeaderTitle's h1 is display: flex, which wraps direct text in an anonymous flex item and suppresses text-overflow: ellipsis. Wrap the text in a block-level span so truncation actually kicks in. */}
							<span className="block min-w-0 truncate">
								{provider.display_name || provider.name}
							</span>
						</PageHeaderTitle>
					</div>
				</PageHeader>
				<div className="border border-solid p-6 rounded-lg">
					<ProviderForm
						editing
						// Use the provider identity as the key so navigating to a
						// different provider remounts the form with fresh values,
						// while background refetches of the same provider don't
						// reset in-progress edits.
						key={provider.id}
						bedrockSavedAccessCredentials={hasBedrockStoredCredentials(
							provider,
						)}
						openAiAnthropicSavedApiKey={openAiAnthropicSavedApiKey}
						openAiAnthropicMaskedApiKey={openAiAnthropicMaskedApiKey}
						initialValues={aiProviderToFormValues(provider)}
						isLoading={updateMutation.isPending}
						submitError={updateMutation.error}
						onSubmit={(values) => {
							const request = providerFormValuesToUpdate(values, provider);
							updateMutation.mutate(request, {
								onSuccess: (updated) => {
									toast.success(
										`Provider "${updated.display_name || updated.name}" updated.`,
									);
								},
								onError: (error) => {
									toast.error(
										getErrorMessage(
											error,
											`Failed to update provider "${provider.display_name || provider.name}".`,
										),
									);
								},
							});
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
