import { RotateCcwIcon } from "lucide-react";
import { type FC, useState } from "react";
import { getErrorMessage } from "#/api/errors";
import type * as TypesGen from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
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
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { ProviderIcon } from "./ChatModelAdminPanel/ProviderIcon";
import {
	TemporarySavedState,
	useTemporarySavedState,
} from "./TemporarySavedState";

interface UserCompactionThresholdSettingsProps {
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	modelConfigsError?: unknown;
	isLoadingModelConfigs?: boolean;
	thresholds: readonly TypesGen.UserChatCompactionThreshold[] | undefined;
	isThresholdsLoading: boolean;
	thresholdsError: unknown;
	onSaveThreshold: (
		modelConfigId: string,
		thresholdPercent: number,
	) => Promise<unknown>;
	onResetThreshold: (modelConfigId: string) => Promise<unknown>;
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

const ContextCompactionHeader: FC = () => (
	<div className="flex flex-col gap-2">
		<h3 className="m-0 text-sm font-semibold text-content-primary">
			Context Compaction
		</h3>
		<p className="!mt-0.5 m-0 text-xs text-content-secondary">
			Control when conversation context is automatically summarized for each
			model. Setting 100% means the conversation will never auto-compact.
		</p>
	</div>
);

export const UserCompactionThresholdSettings: FC<
	UserCompactionThresholdSettingsProps
> = ({
	modelConfigs,
	modelConfigsError,
	isLoadingModelConfigs,
	thresholds,
	isThresholdsLoading,
	thresholdsError,
	onSaveThreshold,
	onResetThreshold,
}) => {
	const [drafts, setDrafts] = useState<Record<string, string>>({});
	const [rowErrors, setRowErrors] = useState<Record<string, string>>({});
	const [pendingModels, setPendingModels] = useState<Set<string>>(new Set());
	const { isSavedVisible, showSavedState } = useTemporarySavedState();

	const enabledModelConfigs = modelConfigs.filter((config) => config.enabled);
	const overridesByModelID = new Map(
		(thresholds ?? []).map(
			(threshold: TypesGen.UserChatCompactionThreshold) => [
				threshold.model_config_id,
				threshold.threshold_percent,
			],
		),
	);

	const clearDraft = (modelConfigID: string) => {
		setDrafts((currentDrafts) => {
			const nextDrafts = { ...currentDrafts };
			delete nextDrafts[modelConfigID];
			return nextDrafts;
		});
	};

	const clearRowError = (modelConfigID: string) => {
		setRowErrors((currentErrors) => {
			if (!(modelConfigID in currentErrors)) {
				return currentErrors;
			}
			const nextErrors = { ...currentErrors };
			delete nextErrors[modelConfigID];
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

	const handleReset = (modelConfigId: string) => {
		clearRowError(modelConfigId);
		addPending(modelConfigId);
		onResetThreshold(modelConfigId)
			.then(() => {
				clearDraft(modelConfigId);
				clearRowError(modelConfigId);
			})
			.catch((error: unknown) => {
				setRowErrors((currentErrors) => ({
					...currentErrors,
					[modelConfigId]: getErrorMessage(
						error,
						"Failed to reset compaction threshold.",
					),
				}));
			})
			.finally(() => {
				removePending(modelConfigId);
			});
	};

	// Compute dirty rows: rows where the user has typed a valid value
	// that differs from the current server-side override.
	const dirtyRows: Array<{ modelConfigId: string; value: number }> = [];
	for (const modelConfig of enabledModelConfigs) {
		const draft = drafts[modelConfig.id];
		if (draft === undefined) continue;
		const parsed = parseThresholdDraft(draft);
		if (parsed === null) continue;
		const existingOverride = overridesByModelID.get(modelConfig.id);
		if (parsed === existingOverride) continue;
		dirtyRows.push({ modelConfigId: modelConfig.id, value: parsed });
	}

	const handleSaveAll = () => {
		const saves = dirtyRows.map(({ modelConfigId, value }) => {
			clearRowError(modelConfigId);
			addPending(modelConfigId);
			return onSaveThreshold(modelConfigId, value)
				.then(() => {
					clearDraft(modelConfigId);
					clearRowError(modelConfigId);
					return true;
				})
				.catch((error: unknown) => {
					setRowErrors((currentErrors) => ({
						...currentErrors,
						[modelConfigId]: getErrorMessage(
							error,
							"Failed to save compaction threshold.",
						),
					}));
					return false;
				})
				.finally(() => {
					removePending(modelConfigId);
				});
		});
		void Promise.all(saves).then((results) => {
			if (results.length > 0 && results.every(Boolean)) {
				showSavedState();
			}
		});
	};

	const handleCancelAll = () => {
		setDrafts({});
		setRowErrors({});
	};

	const hasAnyPending = pendingModels.size > 0;
	const hasAnyErrors = Object.keys(rowErrors).length > 0;
	const hasAnyDrafts = Object.keys(drafts).length > 0;
	const shouldShowActions =
		hasAnyDrafts || hasAnyErrors || hasAnyPending || dirtyRows.length > 0;

	if (isThresholdsLoading) {
		return (
			<div className="flex flex-col gap-2">
				<ContextCompactionHeader />
				<div className="flex items-center gap-2 text-sm text-content-secondary">
					<Spinner loading className="h-4 w-4" />
					Loading thresholds...
				</div>
			</div>
		);
	}

	if (thresholdsError != null) {
		return (
			<div className="flex flex-col gap-2">
				<ContextCompactionHeader />
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
			<ContextCompactionHeader />
			{isLoadingModelConfigs ? (
				<div className="flex items-center gap-2 text-sm text-content-secondary">
					<Spinner loading className="h-4 w-4" />
					Loading models...
				</div>
			) : modelConfigsError ? (
				<p className="m-0 text-xs text-content-destructive">
					{getErrorMessage(
						modelConfigsError,
						"Failed to load model configurations.",
					)}
				</p>
			) : enabledModelConfigs.length === 0 ? (
				<p className="m-0 text-xs text-content-secondary">
					No enabled chat models available. An administrator must configure chat
					models before compaction thresholds can be set.
				</p>
			) : (
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead className="text-content-secondary">Model</TableHead>
							<TableHead className="w-0 whitespace-nowrap">Default</TableHead>
							<TableHead className="w-0 whitespace-nowrap">Threshold</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody>
						{enabledModelConfigs.map((modelConfig) => {
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

							return (
								<TableRow key={modelConfig.id}>
									<TableCell className="text-sm font-medium text-content-primary">
										<Badge size="sm" variant="default" className="w-fit">
											<ProviderIcon
												provider={modelConfig.provider}
												className="h-4 w-4"
											/>
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
										{modelConfig.compression_threshold}%
									</TableCell>
									<TableCell className="w-0 whitespace-nowrap">
										<div className="flex items-center gap-1.5">
											<Tooltip>
												<TooltipTrigger asChild>
													<div className="relative">
														<Input
															aria-label={`${modelName} compaction threshold`}
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
														aria-label={`Reset ${modelName} to default`}
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
														disabled={hasAnyPending}
														onClick={handleSaveAll}
													>
														{hasAnyPending && (
															<Spinner loading className="h-4 w-4" />
														)}
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
			)}
		</div>
	);
};
