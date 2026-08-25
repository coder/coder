import { cn } from "cn";
import { RotateCcwIcon, TriangleAlertIcon } from "lucide-react";
import { type FC, useState } from "react";
import { getErrorMessage } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
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
	TableFooter,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import {
	TemporarySavedState,
	useTemporarySavedState,
} from "#/components/TemporarySavedState/TemporarySavedState";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { formatProviderLabel } from "#/utils/aiProviders";
import {
	bindingCompactionTrigger,
	compactionPointAsPercent,
	compactionTriggerPoint,
	type OrganizationCompactionTrigger,
} from "../compactionTriggers";
import { ProviderIcon } from "./ChatModelAdminPanel/ProviderIcon";

interface UserCompactionThresholdSettingsProps {
	models: readonly TypesGen.ChatModel[];
	providerTypeByID: ReadonlyMap<string, string>;
	organizations: readonly TypesGen.Organization[];
	compactionTriggersByOrganizationID: ReadonlyMap<
		string,
		OrganizationCompactionTrigger
	>;
	modelsError?: unknown;
	compactionTriggersError?: unknown;
	isLoadingModels?: boolean;
	thresholds: readonly TypesGen.UserChatCompactionThreshold[] | undefined;
	isThresholdsLoading: boolean;
	thresholdsError: unknown;
	onSaveThreshold: (
		modelId: string,
		thresholdPercent: number,
	) => Promise<unknown>;
	onResetThreshold: (modelId: string) => Promise<unknown>;
}

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

const ContextCompactionHeader: FC<{
	hasOrganizationCompactionOverride: boolean;
}> = ({ hasOrganizationCompactionOverride }) => (
	<div className="flex flex-col gap-2">
		<h3 className="m-0 text-sm font-semibold text-content-primary">
			Context compaction
		</h3>
		<p className="mt-0.5! m-0 text-xs text-content-secondary">
			Control when conversation context is automatically summarized for each
			model. Setting 100% disables that model&apos;s compaction trigger.
			{hasOrganizationCompactionOverride &&
				" An organization compaction model may still trigger compaction."}
		</p>
	</div>
);

export const UserCompactionThresholdSettings: FC<
	UserCompactionThresholdSettingsProps
> = ({
	models,
	providerTypeByID,
	organizations,
	compactionTriggersByOrganizationID,
	modelsError,
	compactionTriggersError,
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
	const hasOrganizationCompactionOverride =
		compactionTriggersByOrganizationID.size > 0;

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

	if (isThresholdsLoading) {
		return (
			<div className="flex flex-col gap-2">
				<ContextCompactionHeader
					hasOrganizationCompactionOverride={hasOrganizationCompactionOverride}
				/>
				<div className="flex items-center gap-2 text-sm text-content-secondary">
					<Spinner loading className="size-4" />
					Loading thresholds...
				</div>
			</div>
		);
	}

	if (thresholdsError != null) {
		return (
			<div className="flex flex-col gap-2">
				<ContextCompactionHeader
					hasOrganizationCompactionOverride={hasOrganizationCompactionOverride}
				/>
				<p className="m-0 text-xs text-content-destructive">
					{getErrorMessage(
						thresholdsError,
						"Failed to load compaction thresholds.",
					)}
				</p>
			</div>
		);
	}

	return (
		<div className="flex flex-col gap-3">
			<ContextCompactionHeader
				hasOrganizationCompactionOverride={hasOrganizationCompactionOverride}
			/>
			{isLoadingModels ? (
				<div className="flex items-center gap-2 text-sm text-content-secondary">
					<Spinner loading className="size-4" />
					Loading models...
				</div>
			) : modelsError && enabledModels.length === 0 ? (
				<p className="m-0 text-xs text-content-destructive">
					{getErrorMessage(modelsError, "Failed to load model configurations.")}
				</p>
			) : enabledModels.length === 0 ? (
				<p className="m-0 text-xs text-content-secondary">
					No enabled chat models available. An administrator must configure chat
					models before compaction thresholds can be set.
				</p>
			) : (
				<>
					{modelsError && (
						<p className="m-0 text-xs text-content-destructive">
							{getErrorMessage(
								modelsError,
								"Some organization models could not be loaded.",
							)}
						</p>
					)}
					{compactionTriggersError != null && (
						<p className="m-0 text-xs text-content-destructive">
							{getErrorMessage(
								compactionTriggersError,
								"Failed to load organization compaction settings. Warnings about earlier compaction may be missing.",
							)}
						</p>
					)}
					{organizationOptions.length > 1 && activeOrganization && (
						<div>
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
						</div>
					)}
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead className="text-content-secondary">Model</TableHead>
								<TableHead className="w-0 whitespace-nowrap">Default</TableHead>
								<TableHead className="w-0 whitespace-nowrap">
									Threshold
								</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{visibleModels.map((modelConfig) => {
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
								const effectiveThresholdPercent =
									parsedDraftValue ??
									(draftValue.length === 0
										? modelConfig.compression_threshold
										: undefined);
								const organizationTrigger =
									compactionTriggersByOrganizationID.get(
										modelConfig.organization_id,
									);
								const chatTrigger =
									effectiveThresholdPercent === undefined
										? undefined
										: {
												thresholdPercent: effectiveThresholdPercent,
												contextLimit: modelConfig.context_limit,
											};
								const organizationTriggerPercent = organizationTrigger
									? compactionPointAsPercent(
											organizationTrigger.point,
											modelConfig.context_limit,
										)
									: undefined;
								const isOrganizationTriggerEarlier =
									chatTrigger !== undefined &&
									organizationTrigger !== undefined &&
									organizationTriggerPercent !== undefined &&
									bindingCompactionTrigger(
										chatTrigger,
										organizationTrigger.trigger,
									) === "organization" &&
									organizationTrigger.point <
										compactionTriggerPoint(chatTrigger);

								return (
									<TableRow key={modelConfig.id}>
										<TableCell className="text-sm font-medium text-content-primary">
											<Badge
												size="md"
												variant="default"
												className="w-fit"
												aria-label={`${providerLabel} ${modelName} in ${organizationName}`}
											>
												<ProviderIcon provider={provider} className="size-4" />
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
											{isOrganizationTriggerEarlier && organizationTrigger && (
												<p
													role="status"
													className="m-0 mt-1 flex max-w-xl items-start gap-1 text-2xs font-normal text-content-warning"
												>
													<TriangleAlertIcon
														aria-hidden
														className="mt-px size-3 shrink-0"
													/>
													<span>
														Compaction will trigger earlier at approximately{" "}
														{organizationTriggerPercent?.toLocaleString(
															"en-US",
															{
																maximumFractionDigits: 1,
															},
														)}
														% of this model&apos;s window because the
														organization compaction model{" "}
														{organizationTrigger.model.display_name.trim() ||
															organizationTrigger.model.model}{" "}
														compacts at{" "}
														{organizationTrigger.model.compression_threshold}%
														of its{" "}
														{organizationTrigger.model.context_limit.toLocaleString(
															"en-US",
														)}
														-token window.
													</span>
												</p>
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
																: organizationTrigger
																	? "Setting 100% disables this model's compaction trigger. The organization compaction model may still trigger compaction."
																	: "Setting 100% disables this model's compaction trigger."}
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
													Setting 100% disables this model&apos;s compaction
													trigger.
													{organizationTrigger &&
														" The organization compaction model may still trigger compaction."}
												</span>
											)}
										</TableCell>
									</TableRow>
								);
							})}
						</TableBody>
						<TableFooter className="bg-transparent">
							<TableRow className="border-0">
								<TableCell colSpan={3} className="border-0 p-0">
									<div className="mt-2 flex h-6 items-center justify-end gap-2 px-3">
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
								</TableCell>
							</TableRow>
						</TableFooter>
					</Table>
				</>
			)}
		</div>
	);
};
