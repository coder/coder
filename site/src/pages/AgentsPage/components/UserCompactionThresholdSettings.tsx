import { cn } from "cn";
import { RotateCcwIcon, TriangleAlertIcon } from "lucide-react";
import { useState } from "react";
import { getErrorMessage } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	HelpPopover,
	HelpPopoverContent,
	HelpPopoverIconTrigger,
	HelpPopoverText,
	HelpPopoverTitle,
} from "#/components/HelpPopover/HelpPopover";
import { Input } from "#/components/Input/Input";
import {
	getOrganizationLabel,
	OrganizationAutocomplete,
} from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import {
	TemporarySavedState,
	useTemporarySavedState,
} from "#/components/TemporarySavedState/TemporarySavedState";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { formatContextLimit } from "#/modules/aiModels/ModelSelector";
import { ProviderIcon } from "#/modules/aiModels/ProviderIcon";
import { formatProviderLabel } from "#/utils/aiProviders";
import {
	bindingCompactionTriggerPoint,
	bindingCompactionTriggerSource,
	type CompactionTrigger,
	type CompactionTriggerLoadError,
	compactionDisabledThresholdPercent,
	compactionPointAsPercent,
	formatCompactionPercent,
	isCompactionPointBeyondWindow,
	type OrganizationCompactionTrigger,
} from "../compactionTriggers";
import { getModelLabel } from "../utils/modelOptions";

type UserCompactionThresholdSettingsProps = {
	models: readonly TypesGen.ChatModel[];
	providerTypeByID: ReadonlyMap<string, string>;
	organizations: readonly TypesGen.Organization[];
	compactionTriggersByOrganizationID: ReadonlyMap<
		string,
		OrganizationCompactionTrigger
	>;
	modelsError?: unknown;
	compactionTriggerLoadErrors: readonly CompactionTriggerLoadError[];
	isLoadingModels: boolean;
	thresholds: readonly TypesGen.UserChatCompactionThreshold[] | undefined;
	isThresholdsLoading: boolean;
	thresholdsError: unknown;
	onSaveThreshold: (
		modelId: string,
		thresholdPercent: number,
	) => Promise<unknown>;
	onResetThreshold: (modelId: string) => Promise<unknown>;
};

const parseThresholdDraft = (value: string): number | null => {
	const trimmedValue = value.trim();
	if (!/^\d+$/.test(trimmedValue)) {
		return null;
	}

	const parsedValue = Number(trimmedValue);
	if (!Number.isInteger(parsedValue) || parsedValue < 0 || parsedValue > 100) {
		return null;
	}

	return parsedValue;
};

type ContextCompactionHeaderProps = {
	mayHaveOrganizationCompactionOverride: boolean;
};

const ContextCompactionHeader: React.FC<ContextCompactionHeaderProps> = ({
	mayHaveOrganizationCompactionOverride,
}) => (
	<div className="flex flex-col gap-2">
		<h3 className="m-0 text-sm font-semibold text-content-primary">
			Context compaction
		</h3>
		<p className="mt-0.5! m-0 text-xs text-content-secondary">
			Control when conversation context is automatically summarized for each
			model.{" "}
			{mayHaveOrganizationCompactionOverride
				? "Setting 100% turns off that model's own compaction threshold. An organization override may still compact chats with that model."
				: "Setting 100% turns off automatic compaction for that model."}
		</p>
	</div>
);

type CompactionContextCellProps = {
	contextLimit: number;
	chatTrigger: CompactionTrigger | undefined;
	organizationTrigger: OrganizationCompactionTrigger | undefined;
};

const CompactionContextCell: React.FC<CompactionContextCellProps> = ({
	contextLimit,
	chatTrigger,
	organizationTrigger,
}) => {
	const compactionPoint =
		chatTrigger &&
		bindingCompactionTriggerPoint(chatTrigger, organizationTrigger);
	const isCompactionPointReachable =
		compactionPoint !== undefined &&
		!isCompactionPointBeyondWindow(compactionPoint, contextLimit);

	return (
		<TableCell className="w-0 whitespace-nowrap tabular-nums">
			<div className="flex flex-col">
				{contextLimit > 0 ? (
					<span>{formatContextLimit(contextLimit)} tokens</span>
				) : (
					<span className="text-content-secondary">Unknown</span>
				)}
				{isCompactionPointReachable && (
					<span className="text-2xs text-content-secondary">
						Compacts at ~{formatContextLimit(compactionPoint)}
					</span>
				)}
			</div>
		</TableCell>
	);
};

type EffectiveCompactionThresholdProps = {
	modelConfig: TypesGen.ChatModel;
	chatTrigger: CompactionTrigger | undefined;
	organizationTrigger: OrganizationCompactionTrigger | undefined;
	organizationTriggerPercentLabel: string | undefined;
	isOrganizationPointBeyondWindow: boolean;
};

const EffectiveCompactionThreshold: React.FC<
	EffectiveCompactionThresholdProps
> = ({
	modelConfig,
	chatTrigger,
	organizationTrigger,
	organizationTriggerPercentLabel,
	isOrganizationPointBeyondWindow,
}) => {
	const off = <span className="text-content-secondary">Off</span>;

	if (
		chatTrigger !== undefined &&
		organizationTrigger !== undefined &&
		organizationTriggerPercentLabel !== undefined &&
		bindingCompactionTriggerSource(chatTrigger, organizationTrigger.trigger) ===
			"organization"
	) {
		const organizationModelName = getModelLabel(organizationTrigger.model);
		const organizationWindowLabel =
			organizationTrigger.model.context_limit.toLocaleString("en-US");

		return (
			<TableCell className="w-0 whitespace-nowrap tabular-nums">
				<div className="flex items-center gap-1">
					{isOrganizationPointBeyondWindow ? (
						off
					) : (
						<span>{organizationTriggerPercentLabel}%</span>
					)}
					<HelpPopover>
						<HelpPopoverIconTrigger
							size="small"
							hoverEffect={isOrganizationPointBeyondWindow}
							aria-label={`Organization override for ${getModelLabel(modelConfig)}`}
							className={cn(
								!isOrganizationPointBeyondWindow && "text-content-warning",
							)}
						>
							{isOrganizationPointBeyondWindow ? undefined : (
								<TriangleAlertIcon />
							)}
						</HelpPopoverIconTrigger>
						<HelpPopoverContent>
							<HelpPopoverTitle>Organization override</HelpPopoverTitle>
							<HelpPopoverText>
								{isOrganizationPointBeyondWindow ? (
									<>
										The organization override compacts chats at{" "}
										{organizationTrigger.pointTokens.toLocaleString("en-US")}{" "}
										tokens ({organizationTrigger.trigger.thresholdPercent}% of{" "}
										{organizationModelName}&apos;s {organizationWindowLabel}
										-token window), beyond this model&apos;s{" "}
										{modelConfig.context_limit.toLocaleString("en-US")}
										-token window. Chats with this model do not compact
										automatically. Set a threshold below 100% to turn compaction
										back on.
									</>
								) : (
									<>
										The organization override compacts chats at{" "}
										{organizationTrigger.trigger.thresholdPercent}% of{" "}
										{organizationModelName}&apos;s {organizationWindowLabel}
										-token window, about {organizationTriggerPercentLabel}% of
										this model&apos;s window.
									</>
								)}
							</HelpPopoverText>
						</HelpPopoverContent>
					</HelpPopover>
				</div>
			</TableCell>
		);
	}

	return (
		<TableCell className="w-0 whitespace-nowrap tabular-nums">
			{chatTrigger &&
				(chatTrigger.thresholdPercent >= compactionDisabledThresholdPercent
					? off
					: `${chatTrigger.thresholdPercent}%`)}
		</TableCell>
	);
};

type CompactionThresholdRowProps = {
	modelConfig: TypesGen.ChatModel;
	existingOverride: number | undefined;
	draft: string | undefined;
	isThisModelMutating: boolean;
	rowError: string | undefined;
	provider: string;
	organizationName: string;
	organizationTrigger: OrganizationCompactionTrigger | undefined;
	organizationOverrideNotLoaded: boolean;
	onDraftChange: (value: string) => void;
	onReset: () => void;
};

const CompactionThresholdRow: React.FC<CompactionThresholdRowProps> = ({
	modelConfig,
	existingOverride,
	draft,
	isThisModelMutating,
	rowError,
	provider,
	organizationName,
	organizationTrigger,
	organizationOverrideNotLoaded,
	onDraftChange,
	onReset,
}) => {
	const hasOverride = existingOverride !== undefined;
	const draftValue =
		draft ?? (existingOverride !== undefined ? String(existingOverride) : "");
	const parsedDraftValue = parseThresholdDraft(draftValue);
	const isInvalid = draftValue.length > 0 && parsedDraftValue === null;
	// Only warn when user-typed, not when loaded from the server.
	const isDraftDisablingCompaction =
		draftValue === String(compactionDisabledThresholdPercent) &&
		draft !== undefined;
	// A point past this window can only bind while the chat trigger is off,
	// so no trigger fires within this window.
	const isOrganizationPointBeyondWindow =
		organizationTrigger !== undefined &&
		isCompactionPointBeyondWindow(
			organizationTrigger.pointTokens,
			modelConfig.context_limit,
		);
	const organizationTriggerPercent =
		organizationTrigger &&
		compactionPointAsPercent(
			organizationTrigger.pointTokens,
			modelConfig.context_limit,
		);
	const organizationTriggerPercentLabel =
		organizationTriggerPercent === undefined
			? undefined
			: formatCompactionPercent(organizationTriggerPercent);

	let disablingCompactionWarning =
		"Setting 100% turns off automatic compaction for this model.";
	if (
		organizationTriggerPercentLabel !== undefined &&
		!isOrganizationPointBeyondWindow
	) {
		disablingCompactionWarning = `Setting 100% turns off this model's own compaction threshold. Chats still compact at about ${organizationTriggerPercentLabel}% of this model's window, set by the organization override.`;
	} else if (organizationOverrideNotLoaded) {
		disablingCompactionWarning =
			"Setting 100% turns off this model's own compaction threshold. An organization override may still compact chats with this model.";
	}

	const modelName = getModelLabel(modelConfig);
	const providerLabel = formatProviderLabel(provider);
	const effectiveThresholdPercent =
		parsedDraftValue ??
		(draftValue.length === 0 ? modelConfig.compression_threshold : undefined);
	const chatTrigger =
		effectiveThresholdPercent === undefined
			? undefined
			: {
					thresholdPercent: effectiveThresholdPercent,
					contextLimit: modelConfig.context_limit,
				};

	return (
		<TableRow>
			<TableCell className="text-sm font-medium text-content-primary">
				<Badge
					size="md"
					variant="default"
					className="w-fit"
					aria-label={`${providerLabel} ${modelName} in ${organizationName}`}
				>
					<span className="flex size-4 shrink-0 items-center justify-center rounded-full bg-surface-secondary">
						<ProviderIcon
							provider={provider}
							className="size-3/5 text-content-secondary"
						/>
					</span>
					{modelName}
				</Badge>
				{rowError && (
					<p
						aria-live="polite"
						className="m-0 mt-0.5 text-2xs font-normal text-content-destructive"
					>
						{rowError}
					</p>
				)}
			</TableCell>
			<CompactionContextCell
				contextLimit={modelConfig.context_limit}
				chatTrigger={chatTrigger}
				organizationTrigger={organizationTrigger}
			/>
			<TableCell className="w-0 whitespace-nowrap tabular-nums">
				{modelConfig.compression_threshold}%
			</TableCell>
			<TableCell className="w-0 whitespace-nowrap">
				<div className="flex items-center gap-1.5">
					<Tooltip>
						<TooltipTrigger asChild>
							<div className="relative">
								<Input
									aria-label={`${modelName} compaction threshold for ${organizationName}`}
									aria-invalid={isInvalid || undefined}
									type="text"
									min={0}
									max={100}
									maxLength={3}
									inputMode="numeric"
									className={cn(
										"h-7 w-16 px-2 pr-5 text-xs tabular-nums",
										isInvalid &&
											"border-content-destructive focus:ring-content-destructive/30",
									)}
									value={draftValue}
									placeholder={String(modelConfig.compression_threshold)}
									onChange={(event) => onDraftChange(event.target.value)}
									disabled={isThisModelMutating}
								/>
								<span
									aria-hidden="true"
									className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 text-xs text-content-secondary"
								>
									%
								</span>
							</div>
						</TooltipTrigger>
						{(isInvalid || isDraftDisablingCompaction) && (
							<TooltipContent>
								{isInvalid
									? "Enter a whole number between 0 and 100."
									: disablingCompactionWarning}
							</TooltipContent>
						)}
					</Tooltip>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								size="icon"
								variant="subtle"
								className={cn(
									"size-7",
									hasOverride ? "opacity-100" : "pointer-events-none opacity-0",
								)}
								aria-label={`Reset ${modelName} for ${organizationName} to default`}
								aria-hidden={!hasOverride}
								tabIndex={hasOverride ? 0 : -1}
								disabled={isThisModelMutating || !hasOverride}
								onClick={onReset}
							>
								<RotateCcwIcon className="size-3.5" />
							</Button>
						</TooltipTrigger>
						{hasOverride && (
							<TooltipContent>
								Reset to default ({modelConfig.compression_threshold}%)
							</TooltipContent>
						)}
					</Tooltip>
				</div>
				{isInvalid && (
					<span className="sr-only" aria-live="polite">
						Enter a whole number between 0 and 100.
					</span>
				)}
				{isDraftDisablingCompaction && (
					<span className="sr-only" aria-live="polite">
						{disablingCompactionWarning}
					</span>
				)}
			</TableCell>
			<EffectiveCompactionThreshold
				modelConfig={modelConfig}
				chatTrigger={chatTrigger}
				organizationTrigger={organizationTrigger}
				organizationTriggerPercentLabel={organizationTriggerPercentLabel}
				isOrganizationPointBeyondWindow={isOrganizationPointBeyondWindow}
			/>
		</TableRow>
	);
};

export const UserCompactionThresholdSettings: React.FC<
	UserCompactionThresholdSettingsProps
> = ({
	models,
	providerTypeByID,
	organizations,
	compactionTriggersByOrganizationID,
	modelsError,
	compactionTriggerLoadErrors,
	isLoadingModels,
	thresholds,
	isThresholdsLoading,
	thresholdsError,
	onSaveThreshold,
	onResetThreshold,
}) => {
	const [drafts, setDrafts] = useState<Record<string, string>>({});
	const [rowErrors, setRowErrors] = useState<Record<string, string>>({});
	const [pendingModels, setPendingModels] = useState<Set<string>>(new Set());
	const [selectedOrganizationID, setSelectedOrganizationID] = useState<
		string | null
	>(null);
	const { isSavedVisible, showSavedState } = useTemporarySavedState();

	const enabledModels = models.filter((config) => config.enabled);
	const organizationNameByID = new Map(
		organizations.map((organization) => [
			organization.id,
			organization.display_name || organization.name,
		]),
	);
	const organizationIDsWithLoadErrors = new Set(
		compactionTriggerLoadErrors.map(({ organizationID }) => organizationID),
	);
	const organizationOptions = organizations.filter((organization) =>
		enabledModels.some((config) => config.organization_id === organization.id),
	);
	const activeOrganization =
		organizationOptions.find(
			(organization) => organization.id === selectedOrganizationID,
		) ??
		organizationOptions.find((organization) => organization.is_default) ??
		organizationOptions[0];
	const visibleModels = activeOrganization
		? enabledModels.filter(
				(config) => config.organization_id === activeOrganization.id,
			)
		: enabledModels;
	const overridesByModelID = new Map(
		(thresholds ?? []).map(
			(threshold: TypesGen.UserChatCompactionThreshold) => [
				threshold.model_config_id,
				threshold.threshold_percent,
			],
		),
	);

	const clearDraft = (modelID: string) => {
		setDrafts((currentDrafts) => {
			const nextDrafts = { ...currentDrafts };
			delete nextDrafts[modelID];
			return nextDrafts;
		});
	};

	const clearRowError = (modelID: string) => {
		setRowErrors((currentErrors) => {
			if (!(modelID in currentErrors)) {
				return currentErrors;
			}
			const nextErrors = { ...currentErrors };
			delete nextErrors[modelID];
			return nextErrors;
		});
	};

	const addPending = (id: string) => {
		setPendingModels((pending) => new Set(pending).add(id));
	};

	const removePending = (id: string) => {
		setPendingModels((pending) => {
			const next = new Set(pending);
			next.delete(id);
			return next;
		});
	};

	const handleReset = (modelId: string) => {
		clearRowError(modelId);
		addPending(modelId);
		onResetThreshold(modelId)
			.then(() => {
				clearDraft(modelId);
				clearRowError(modelId);
			})
			.catch((error: unknown) => {
				setRowErrors((currentErrors) => ({
					...currentErrors,
					[modelId]: getErrorMessage(
						error,
						"Failed to reset compaction threshold.",
					),
				}));
			})
			.finally(() => {
				removePending(modelId);
			});
	};

	// Save/cancel act only on visible rows; drafts hidden by the org
	// picker are kept untouched.
	const visibleModelIDs = new Set(visibleModels.map((config) => config.id));
	const dirtyRows: Array<{ modelId: string; value: number }> = [];
	for (const modelConfig of visibleModels) {
		const draft = drafts[modelConfig.id];
		if (draft === undefined) continue;
		const parsed = parseThresholdDraft(draft);
		if (parsed === null) continue;
		const existingOverride = overridesByModelID.get(modelConfig.id);
		if (parsed === existingOverride) continue;
		dirtyRows.push({ modelId: modelConfig.id, value: parsed });
	}

	const handleSaveAll = () => {
		const saves = dirtyRows.map(({ modelId, value }) => {
			clearRowError(modelId);
			addPending(modelId);
			return onSaveThreshold(modelId, value)
				.then(() => {
					clearDraft(modelId);
					clearRowError(modelId);
					return true;
				})
				.catch((error: unknown) => {
					setRowErrors((currentErrors) => ({
						...currentErrors,
						[modelId]: getErrorMessage(
							error,
							"Failed to save compaction threshold.",
						),
					}));
					return false;
				})
				.finally(() => {
					removePending(modelId);
				});
		});
		void Promise.all(saves).then((results) => {
			if (results.length > 0 && results.every(Boolean)) {
				showSavedState();
			}
		});
	};

	const handleCancelAll = () => {
		setDrafts((currentDrafts) =>
			Object.fromEntries(
				Object.entries(currentDrafts).filter(
					([modelID]) => !visibleModelIDs.has(modelID),
				),
			),
		);
		setRowErrors((currentErrors) =>
			Object.fromEntries(
				Object.entries(currentErrors).filter(
					([modelID]) => !visibleModelIDs.has(modelID),
				),
			),
		);
	};

	const hasAnyPending = [...pendingModels].some((modelID) =>
		visibleModelIDs.has(modelID),
	);
	const hasAnyErrors = Object.keys(rowErrors).some((modelID) =>
		visibleModelIDs.has(modelID),
	);
	const hasAnyDrafts = Object.keys(drafts).some((modelID) =>
		visibleModelIDs.has(modelID),
	);
	const shouldShowActions =
		hasAnyDrafts || hasAnyErrors || hasAnyPending || dirtyRows.length > 0;
	const isTableLoading = isThresholdsLoading || isLoadingModels;
	const showRows =
		!isTableLoading && thresholdsError == null && enabledModels.length > 0;
	// A failed load is the alert above the table. The in-table empty state is
	// only for a successful load with no enabled models.
	const blockingError =
		thresholdsError != null ||
		(!isLoadingModels && modelsError != null && enabledModels.length === 0);

	return (
		<div className="flex flex-col gap-4">
			<ContextCompactionHeader
				mayHaveOrganizationCompactionOverride={
					isLoadingModels ||
					compactionTriggersByOrganizationID.size > 0 ||
					compactionTriggerLoadErrors.length > 0
				}
			/>
			{thresholdsError != null && <ErrorAlert error={thresholdsError} />}
			{modelsError != null && <ErrorAlert error={modelsError} />}
			{showRows && compactionTriggerLoadErrors.length > 0 && (
				<p className="m-0 text-xs text-content-destructive">
					{getErrorMessage(
						compactionTriggerLoadErrors[0]?.error,
						"Failed to load organization compaction settings.",
					)}
					<span className="block">
						Effective values in{" "}
						{compactionTriggerLoadErrors
							.map(
								({ organizationID }) =>
									organizationNameByID.get(organizationID) ?? organizationID,
							)
							.join(", ")}{" "}
						ignore the organization override. Reload the page to retry.
					</span>
				</p>
			)}
			{showRows && organizationOptions.length > 1 && activeOrganization && (
				<OrganizationAutocomplete
					value={activeOrganization}
					ariaLabel={`Organization ${getOrganizationLabel(
						activeOrganization,
						organizationOptions,
					)}`}
					options={organizationOptions}
					triggerClassName="w-60"
					optionsTabbable
					onChange={(organization) => {
						if (!organization) {
							return;
						}
						setSelectedOrganizationID(organization.id);
					}}
				/>
			)}
			{!blockingError && (
				<Table aria-label="Compaction thresholds">
					<TableHeader>
						<TableRow>
							<TableHead>Model</TableHead>
							<TableHead className="whitespace-nowrap">Context</TableHead>
							<TableHead className="whitespace-nowrap">Default</TableHead>
							<TableHead className="whitespace-nowrap">Threshold</TableHead>
							<TableHead className="whitespace-nowrap">Effective</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{isTableLoading ? (
							<TableLoader />
						) : enabledModels.length === 0 ? (
							<TableEmpty
								message="No enabled chat models"
								description="An administrator must configure chat models before compaction thresholds can be set."
							/>
						) : (
							visibleModels.map((modelConfig) => (
								<CompactionThresholdRow
									key={modelConfig.id}
									modelConfig={modelConfig}
									existingOverride={overridesByModelID.get(modelConfig.id)}
									draft={drafts[modelConfig.id]}
									isThisModelMutating={pendingModels.has(modelConfig.id)}
									rowError={rowErrors[modelConfig.id]}
									provider={
										providerTypeByID.get(modelConfig.ai_provider_id) ?? ""
									}
									organizationName={
										organizationNameByID.get(modelConfig.organization_id) ??
										modelConfig.organization_id
									}
									organizationTrigger={compactionTriggersByOrganizationID.get(
										modelConfig.organization_id,
									)}
									organizationOverrideNotLoaded={organizationIDsWithLoadErrors.has(
										modelConfig.organization_id,
									)}
									onDraftChange={(value) => {
										setDrafts((currentDrafts) => ({
											...currentDrafts,
											[modelConfig.id]: value,
										}));
										clearRowError(modelConfig.id);
									}}
									onReset={() => handleReset(modelConfig.id)}
								/>
							))
						)}
					</TableBody>
				</Table>
			)}
			{showRows && (
				<div className="flex h-6 items-center justify-end gap-2">
					{isSavedVisible ? (
						<TemporarySavedState />
					) : (
						shouldShowActions && (
							<>
								<Button
									size="xs"
									variant="outline"
									type="button"
									onClick={handleCancelAll}
									disabled={hasAnyPending}
								>
									Cancel
								</Button>
								{dirtyRows.length > 0 && (
									<Button
										size="xs"
										type="button"
										className="h-6"
										disabled={hasAnyPending}
										onClick={handleSaveAll}
									>
										{hasAnyPending && <Spinner loading size="sm" />}
										{hasAnyPending
											? "Saving..."
											: `Save ${dirtyRows.length} ${dirtyRows.length === 1 ? "change" : "changes"}`}
									</Button>
								)}
							</>
						)
					)}
				</div>
			)}
		</div>
	);
};
