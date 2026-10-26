import { useFormik } from "formik";
import { Link as RouterLink } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Link } from "#/components/Link/Link";
import {
	Select,
	SelectContent,
	SelectGroup,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { useTemporarySavedState } from "#/components/TemporarySavedState/TemporarySavedState";
import { AgentSettingLayout } from "#/pages/AISettingsPage/CoderAgentsPage/components/AgentSettingLayout";

type MutationCallbacks = {
	onSuccess?: () => void;
	onError?: () => void;
};

type VirtualDesktopSettingsProps = {
	computerUseProviderData: TypesGen.ChatComputerUseProviderResponse | undefined;
	isLoadingComputerUseProvider: boolean;
	aiProviders: readonly TypesGen.AIProvider[] | undefined;
	isLoadingAIProviders: boolean;
	aiProvidersError: unknown;
	onSaveComputerUseProvider: (
		req: TypesGen.UpdateChatComputerUseProviderRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingComputerUseProvider: boolean;
	computerUseProviderSaveError: Error | null;
};

const computerUseProviderLabels: Record<
	TypesGen.ChatComputerUseProvider,
	string
> = {
	anthropic: "Anthropic",
	openai: "OpenAI",
};

const isComputerUseProviderType = (
	type: TypesGen.AIProviderType,
): type is TypesGen.ChatComputerUseProvider =>
	Object.hasOwn(computerUseProviderLabels, type);

type ComputerUseAIProvider = {
	id: string;
	name: string;
	type: TypesGen.ChatComputerUseProvider;
};

const getComputerUseAIProviders = (
	providers: readonly TypesGen.AIProvider[],
): ComputerUseAIProvider[] =>
	providers.flatMap((provider) =>
		provider.enabled && isComputerUseProviderType(provider.type)
			? [
					{
						id: provider.id,
						name: provider.display_name || provider.name,
						type: provider.type,
					},
				]
			: [],
	);

export const VirtualDesktopSettings: React.FC<VirtualDesktopSettingsProps> = ({
	computerUseProviderData,
	isLoadingComputerUseProvider,
	aiProviders,
	isLoadingAIProviders,
	aiProvidersError,
	onSaveComputerUseProvider,
	isSavingComputerUseProvider,
	computerUseProviderSaveError,
}) => {
	const { isSavedVisible, showSavedState } = useTemporarySavedState();
	const storedAIProviderID = computerUseProviderData?.ai_provider_id ?? "";
	const hasLoaded =
		computerUseProviderData !== undefined && aiProviders !== undefined;
	const eligibleProviders = getComputerUseAIProviders(aiProviders ?? []);

	const form = useFormik<{ aiProviderID: string }>({
		enableReinitialize: true,
		initialValues: {
			aiProviderID: storedAIProviderID,
		},
		onSubmit: (values, helpers) => {
			const selected = eligibleProviders.find(
				(provider) => provider.id === values.aiProviderID,
			);
			if (!selected) {
				return;
			}
			onSaveComputerUseProvider(
				{ provider: selected.type, ai_provider_id: selected.id },
				{
					onSuccess: () => {
						showSavedState();
						helpers.resetForm({ values });
					},
				},
			);
		},
	});

	const isLoading = isLoadingComputerUseProvider || isLoadingAIProviders;
	const isFormDisabled =
		isSavingComputerUseProvider ||
		isLoading ||
		!hasLoaded ||
		eligibleProviders.length === 0;
	const canSave = hasLoaded && form.dirty;
	const selectedProvider = eligibleProviders.find(
		(provider) => provider.id === form.values.aiProviderID,
	);

	let hint: React.ReactNode;
	if (aiProvidersError != null) {
		hint = <ErrorAlert error={aiProvidersError} />;
	} else if (!hasLoaded) {
		hint = undefined;
	} else if (eligibleProviders.length === 0) {
		hint = (
			<p className="m-0 text-xs text-content-secondary">
				Add an enabled Anthropic or OpenAI{" "}
				<RouterLink
					to="/ai/settings/providers"
					className="text-content-link transition-colors hover:text-content-link/80"
				>
					AI provider
				</RouterLink>{" "}
				to use computer use.
			</p>
		);
	} else if (storedAIProviderID === "") {
		hint = (
			<p className="m-0 text-xs text-content-secondary">
				No provider selected. Computer use uses the first enabled{" "}
				{computerUseProviderLabels[computerUseProviderData.provider]} provider.
			</p>
		);
	} else if (
		!eligibleProviders.some((provider) => provider.id === storedAIProviderID)
	) {
		hint = (
			<p className="m-0 text-xs text-content-secondary">
				The selected provider is disabled or was deleted. Select another
				provider.
			</p>
		);
	}

	return (
		<AgentSettingLayout
			title="Virtual desktop"
			description={
				<>
					Allow agents to use a virtual, graphical desktop within workspaces.
					Requires the{" "}
					<Link
						href="https://registry.coder.com/modules/coder/portabledesktop"
						target="_blank"
						size="sm"
					>
						portabledesktop module
					</Link>{" "}
					to be installed in the workspace and a computer use provider to be
					configured.
				</>
			}
			showSave={canSave}
			isSaving={isSavingComputerUseProvider}
			isSavedVisible={isSavedVisible}
			saveDisabled={isFormDisabled || !canSave}
			onSubmit={form.handleSubmit}
			error={
				computerUseProviderSaveError ? (
					<p className="m-0">Failed to save computer use provider.</p>
				) : undefined
			}
		>
			<div className="flex w-88 max-w-full flex-col gap-2">
				<Select
					value={form.values.aiProviderID}
					onValueChange={(value) =>
						void form.setFieldValue("aiProviderID", value)
					}
					disabled={isFormDisabled}
				>
					<SelectTrigger
						aria-label="Computer use provider"
						className="h-10 w-full justify-between rounded-md border border-border border-solid bg-transparent px-3 text-sm shadow-none"
					>
						<SelectValue placeholder="Select provider">
							{isLoading ? (
								<Skeleton className="h-4 w-20" aria-hidden="true" />
							) : (
								selectedProvider?.name
							)}
						</SelectValue>
					</SelectTrigger>
					<SelectContent align="end" className="min-w-44">
						<SelectGroup>
							{eligibleProviders.map((provider) => (
								<SelectItem key={provider.id} value={provider.id}>
									{provider.name}{" "}
									<span className="ml-1 text-xs text-content-secondary">
										{computerUseProviderLabels[provider.type]}
									</span>
								</SelectItem>
							))}
						</SelectGroup>
					</SelectContent>
				</Select>
				{hint}
			</div>
		</AgentSettingLayout>
	);
};
