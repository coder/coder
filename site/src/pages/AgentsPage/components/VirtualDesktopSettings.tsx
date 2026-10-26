import { useFormik } from "formik";
import { useId, useRef } from "react";
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
	onSettled?: () => void;
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

const getEligibleComputerUseProviders = (
	providers: readonly TypesGen.AIProvider[],
): ComputerUseAIProvider[] => {
	const eligible = providers.flatMap((provider) => {
		const type = ChatComputerUseProviders.find(
			(computerUseType) => computerUseType === provider.type,
		);
		if (!provider.enabled || !type) {
			return [];
		}
		return [
			{ provider, type, baseName: provider.display_name || provider.name },
		];
	});
	return eligible.map(({ provider, type, baseName }) => {
		// Display names are not unique, but provider names are.
		const needsNameSuffix =
			baseName !== provider.name &&
			eligible.some(
				(other) =>
					other.provider.id !== provider.id && other.baseName === baseName,
			);
		return {
			id: provider.id,
			displayName: needsNameSuffix
				? `${baseName} (${provider.name})`
				: baseName,
			typeLabel: formatProviderLabel(type),
			type,
		};
	});
};

type ComputerUseProviderHintKind =
	| "no-eligible-providers"
	| "no-eligible-provider-of-type"
	| "unselected"
	| "unavailable"
	| "type-mismatch";

const getComputerUseProviderHintKind = (
	eligibleProviders: readonly ComputerUseAIProvider[],
	selectedAIProviderID: string,
	selectedProvider: ComputerUseAIProvider | undefined,
	isStoredTypeMismatch: boolean,
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
			: "no-eligible-provider-of-type";
	}
	if (!selectedProvider) {
		return "unavailable";
	}
	return isStoredTypeMismatch ? "type-mismatch" : undefined;
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
					Add an enabled{" "}
					{ChatComputerUseProviders.map(formatProviderLabel).join(" or ")}{" "}
					<Link asChild showExternalIcon={false} size="sm">
						<RouterLink to="/ai/settings/providers">AI provider</RouterLink>
					</Link>{" "}
					to use computer use.
				</>
			)}
			{kind === "no-eligible-provider-of-type" &&
				`No provider selected, and no enabled ${storedTypeLabel} provider exists. Select a provider.`}
			{kind === "unselected" &&
				`No provider selected. Computer use uses the first enabled ${storedTypeLabel} provider.`}
			{kind === "unavailable" &&
				"The selected provider is disabled or was deleted. Select another provider."}
			{kind === "type-mismatch" &&
				`The saved computer use provider is ${storedTypeLabel}, which does not match the selected provider's type, so computer use is unavailable. Save to fix it.`}
		</p>
	);
};

type ComputerUseProviderOptionLabelProps = {
	provider: ComputerUseAIProvider;
};

const ComputerUseProviderOptionLabel: React.FC<
	ComputerUseProviderOptionLabelProps
> = ({ provider }) => {
	if (
		provider.displayName
			.toLowerCase()
			.includes(provider.typeLabel.toLowerCase())
	) {
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
	// Formik does not block a second submit before the saving state
	// re-renders, so several clicks in one tick would each send a request.
	const isSubmittingRef = useRef(false);
	const eligibleProviders = getEligibleComputerUseProviders(aiProviders ?? []);
	const storedAIProviderID = computerUseProviderData?.ai_provider_id ?? "";

	const form = useFormik<{ aiProviderID: string }>({
		enableReinitialize: true,
		initialValues: {
			aiProviderID: storedAIProviderID,
		},
		onSubmit: (values, helpers) => {
			const selected = eligibleProviders.find(
				(provider) => provider.id === values.aiProviderID,
			);
			if (!selected || isSubmittingRef.current) {
				return;
			}
			isSubmittingRef.current = true;
			onSaveComputerUseProvider(
				{ provider: selected.type, ai_provider_id: selected.id },
				{
					onSuccess: () => {
						showSavedState();
						helpers.resetForm({ values });
					},
					onSettled: () => {
						isSubmittingRef.current = false;
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
	const isStoredTypeMismatch =
		computerUseProviderData !== undefined &&
		selectedProvider !== undefined &&
		selectedProvider.id === storedAIProviderID &&
		selectedProvider.type !== computerUseProviderData.provider;
	const canSave =
		selectedProvider !== undefined && (form.dirty || isStoredTypeMismatch);
	const selectValueContent = isLoading ? (
		<Skeleton className="h-4 w-20" aria-hidden="true" />
	) : (
		(selectedProvider?.displayName ?? "Select provider")
	);
	const hintKind =
		computerUseProviderData && aiProviders
			? getComputerUseProviderHintKind(
					eligibleProviders,
					form.values.aiProviderID,
					selectedProvider,
					isStoredTypeMismatch,
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
					onValueChange={(value) => {
						// Radix's hidden form select emits "" for a value with no
						// matching option (disabled or not yet loaded), so ignore it.
						if (eligibleProviders.some((provider) => provider.id === value)) {
							void form.setFieldValue("aiProviderID", value);
						}
					}}
					disabled={isFormDisabled}
				>
					<SelectTrigger
						aria-label="Computer use provider"
						aria-describedby={hintKind ? hintId : undefined}
						className="h-10 w-full justify-between rounded-md border border-border border-solid bg-transparent px-3 text-sm shadow-none"
					>
						<SelectValue placeholder={selectValueContent}>
							{selectValueContent}
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
