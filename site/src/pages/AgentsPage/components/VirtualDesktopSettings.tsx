import { useFormik } from "formik";
import { useId } from "react";
import { Link as RouterLink } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { ChatComputerUseProviders } from "#/api/typesGenerated";
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
import { formatProviderLabel } from "#/utils/aiProviders";

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

type ComputerUseAIProvider = {
	id: string;
	displayName: string;
	typeLabel: string;
	type: TypesGen.ChatComputerUseProvider;
};

const getComputerUseAIProviders = (
	providers: readonly TypesGen.AIProvider[],
): ComputerUseAIProvider[] =>
	providers.flatMap((provider) => {
		const type = ChatComputerUseProviders.find(
			(computerUseType) => computerUseType === provider.type,
		);
		if (!provider.enabled || !type) {
			return [];
		}
		return [
			{
				id: provider.id,
				displayName: provider.display_name || provider.name,
				typeLabel: formatProviderLabel(type),
				type,
			},
		];
	});

const computerUseProviderTypesLabel =
	ChatComputerUseProviders.map(formatProviderLabel).join(" or ");

type ComputerUseProviderHintKind =
	| "no-eligible-providers"
	| "no-provider-of-type"
	| "unselected"
	| "unavailable";

const getComputerUseProviderHintKind = (
	eligibleProviders: readonly ComputerUseAIProvider[],
	selectedAIProviderID: string,
	selectedProvider: ComputerUseAIProvider | undefined,
	storedProviderType: TypesGen.ChatComputerUseProvider,
): ComputerUseProviderHintKind | undefined => {
	if (eligibleProviders.length === 0) {
		return "no-eligible-providers";
	}
	if (selectedAIProviderID === "") {
		return eligibleProviders.some(
			(provider) => provider.type === storedProviderType,
		)
			? "unselected"
			: "no-provider-of-type";
	}
	return selectedProvider ? undefined : "unavailable";
};

type ComputerUseProviderHintProps = {
	id: string;
	kind: ComputerUseProviderHintKind;
	storedProviderType: TypesGen.ChatComputerUseProvider;
};

const ComputerUseProviderHint: React.FC<ComputerUseProviderHintProps> = ({
	id,
	kind,
	storedProviderType,
}) => {
	const storedTypeLabel = formatProviderLabel(storedProviderType);
	return (
		<p id={id} className="m-0 text-xs text-content-secondary">
			{kind === "no-eligible-providers" && (
				<>
					Add an enabled {computerUseProviderTypesLabel}{" "}
					<Link asChild showExternalIcon={false} size="sm">
						<RouterLink to="/ai/settings/providers">AI provider</RouterLink>
					</Link>{" "}
					to use computer use.
				</>
			)}
			{kind === "no-provider-of-type" &&
				`No provider selected, and no enabled ${storedTypeLabel} provider exists. Select a provider.`}
			{kind === "unselected" &&
				`No provider selected. Computer use uses the first enabled ${storedTypeLabel} provider.`}
			{kind === "unavailable" &&
				"The selected provider is disabled or was deleted. Select another provider."}
		</p>
	);
};

type ComputerUseProviderOptionLabelProps = {
	provider: ComputerUseAIProvider;
};

const ComputerUseProviderOptionLabel: React.FC<
	ComputerUseProviderOptionLabelProps
> = ({ provider }) => {
	if (provider.displayName === provider.typeLabel) {
		return provider.displayName;
	}
	return (
		<>
			{provider.displayName}{" "}
			<span className="text-xs text-content-secondary">
				{provider.typeLabel}
			</span>
		</>
	);
};

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
	const hintId = useId();
	const eligibleProviders = getComputerUseAIProviders(aiProviders ?? []);

	const form = useFormik<{ aiProviderID: string }>({
		enableReinitialize: true,
		initialValues: {
			aiProviderID: computerUseProviderData?.ai_provider_id ?? "",
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
	const selectedProvider = eligibleProviders.find(
		(provider) => provider.id === form.values.aiProviderID,
	);
	const isFormDisabled =
		isSavingComputerUseProvider ||
		isLoading ||
		computerUseProviderData === undefined ||
		eligibleProviders.length === 0;
	const canSave = form.dirty && selectedProvider !== undefined;
	const hintKind =
		computerUseProviderData && aiProviders
			? getComputerUseProviderHintKind(
					eligibleProviders,
					form.values.aiProviderID,
					selectedProvider,
					computerUseProviderData.provider,
				)
			: undefined;

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
				{aiProvidersError != null && <ErrorAlert error={aiProvidersError} />}
				<Select
					value={form.values.aiProviderID}
					onValueChange={(value) =>
						void form.setFieldValue("aiProviderID", value)
					}
					disabled={isFormDisabled}
				>
					<SelectTrigger
						aria-label="Computer use provider"
						aria-describedby={hintKind ? hintId : undefined}
						className="h-10 w-full justify-between rounded-md border border-border border-solid bg-transparent px-3 text-sm shadow-none"
					>
						<SelectValue
							placeholder={
								isLoading ? (
									<Skeleton className="h-4 w-20" aria-hidden="true" />
								) : (
									"Select provider"
								)
							}
						>
							{isLoading ? (
								<Skeleton className="h-4 w-20" aria-hidden="true" />
							) : (
								(selectedProvider?.displayName ?? "Select provider")
							)}
						</SelectValue>
					</SelectTrigger>
					<SelectContent align="end" className="min-w-44">
						<SelectGroup>
							{eligibleProviders.map((provider) => (
								<SelectItem key={provider.id} value={provider.id}>
									<ComputerUseProviderOptionLabel provider={provider} />
								</SelectItem>
							))}
						</SelectGroup>
					</SelectContent>
				</Select>
				{hintKind && computerUseProviderData && (
					<ComputerUseProviderHint
						id={hintId}
						kind={hintKind}
						storedProviderType={computerUseProviderData.provider}
					/>
				)}
			</div>
		</AgentSettingLayout>
	);
};
