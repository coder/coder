import { cn } from "cn";
import { RotateCcwIcon } from "lucide-react";
import { useState } from "react";
import { getErrorMessage } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
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
	compactionTriggerTokens,
	resolveCompactionContextLimit,
} from "../utils/modelOptions";

type UserCompactionThresholdSettingsProps = {
	models: readonly TypesGen.ChatModel[];
	providerTypeByID: ReadonlyMap<string, string>;
	organizations: readonly TypesGen.Organization[];
	/**
	 * Organization ID to the model config the organization routes compaction
	 * through. Missing entries mean the chat model summarizes itself.
	 */
	compactionModelIDByOrganization?: ReadonlyMap<string, string>;
	modelsError?: unknown;
	isLoadingModels?: boolean;
	thresholds: readonly TypesGen.UserChatCompactionThreshold[] | undefined;
	isThresholdsLoading: boolean;
	thresholdsError: unknown;
	onSaveThreshold: (
		modelId: string,
		thresholdPercent: number,
	) => Promise<unknown>;
	onResetThreshold: (modelId: string) => Promise<unknown>;
};

const noCompactionOverrides: ReadonlyMap<string, string> = new Map();

const ContextCompactionHeader: React.FC = () => (
	<div className="flex flex-col gap-2">
		<h3 className="m-0 text-sm font-semibold text-content-primary">
			Context compaction
		</h3>
		<p className="mt-0.5! m-0 text-xs text-content-secondary">
			Control when conversation context is automatically summarized for each
			model. Setting 100% means the conversation will never auto-compact.
		</p>
	</div>
);

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

export const UserCompactionThresholdSettings: React.FC<
	UserCompactionThresholdSettingsProps
> = ({
	models,
	providerTypeByID,
	organizations,
	compactionModelIDByOrganization = noCompactionOverrides,
	modelsError,
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
	const isTableLoading = isThresholdsLoading || isLoadingModels === true;
	const showRows =
		!isTableLoading && thresholdsError == null && enabledModels.length > 0;
	// A failed load is the alert above the table. The in-table empty state is
	// only for a successful load with no enabled models.
	const blockingError =
		thresholdsError != null ||
		(!isLoadingModels && modelsError != null && enabledModels.length === 0);

	return (
		<div className="flex flex-col gap-4">
			<ContextCompactionHeader />
			{thresholdsError != null && <ErrorAlert error={thresholdsError} />}
			{modelsError != null && <ErrorAlert error={modelsError} />}
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
							visibleModels.map((modelConfig) => {
								const existingOverride = overridesByModelID.get(modelConfig.id);
								const hasOverride = overridesByModelID.has(modelConfig.id);
								const draftValue =
									drafts[modelConfig.id] ??
									(existingOverride !== undefined
										? String(existingOverride)
										: "");
								const parsedDraftValue = parseThresholdDraft(draftValue);
								const isThisModelMutating = pendingModels.has(modelConfig.id);
								const isInvalid =
									draftValue.length > 0 && parsedDraftValue === null;
								// Only warn when user-typed, not when loaded from
								// the server.
								const isDraftDisablingCompaction =
									draftValue === "100" && drafts[modelConfig.id] !== undefined;
								const rowError = rowErrors[modelConfig.id];
								const modelName = modelConfig.display_name || modelConfig.model;
								const provider =
									providerTypeByID.get(modelConfig.ai_provider_id) ?? "";
								const providerLabel = formatProviderLabel(provider);
								const organizationName =
									organizationNameByID.get(modelConfig.organization_id) ??
									modelConfig.organization_id;
								// Prefer the typed draft so the trigger point tracks
								// what the user is about to save.
								const effectiveThreshold =
									parsedDraftValue ??
									existingOverride ??
									modelConfig.compression_threshold;
								const contextLimit = resolveCompactionContextLimit(
									modelConfig,
									models,
									compactionModelIDByOrganization,
								);
								const triggerTokens = compactionTriggerTokens(
									contextLimit,
									effectiveThreshold,
								);

								return (
									<TableRow key={modelConfig.id}>
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
										<TableCell className="w-0 whitespace-nowrap tabular-nums">
											{contextLimit > 0 ? (
												<div className="flex flex-col">
													<span>{formatContextLimit(contextLimit)} tokens</span>
													{triggerTokens !== undefined && (
														<span className="text-2xs text-content-secondary">
															Compacts at ~{formatContextLimit(triggerTokens)}
														</span>
													)}
												</div>
											) : (
												<span className="text-content-secondary">Unknown</span>
											)}
										</TableCell>
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
																placeholder={String(
																	modelConfig.compression_threshold,
																)}
																onChange={(event) => {
																	setDrafts((currentDrafts) => ({
																		...currentDrafts,
																		[modelConfig.id]: event.target.value,
																	}));
																	clearRowError(modelConfig.id);
																}}
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
																: "Setting 100% will disable auto-compaction for this model."}
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
																hasOverride
																	? "opacity-100"
																	: "pointer-events-none opacity-0",
															)}
															aria-label={`Reset ${modelName} for ${organizationName} to default`}
															aria-hidden={!hasOverride}
															tabIndex={hasOverride ? 0 : -1}
															disabled={isThisModelMutating || !hasOverride}
															onClick={() => handleReset(modelConfig.id)}
														>
															<RotateCcwIcon className="size-3.5" />
														</Button>
													</TooltipTrigger>
													{hasOverride && (
														<TooltipContent>
															Reset to default (
															{modelConfig.compression_threshold}%)
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
													Setting 100% will disable auto-compaction for this
													model.
												</span>
											)}
										</TableCell>
									</TableRow>
								);
							})
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
