import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { getErrorMessage } from "#/api/errors";
import {
	deleteUserCompactionThreshold,
	updateUserCompactionThreshold,
	userCompactionThresholds,
} from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Spinner } from "#/components/Spinner/Spinner";

interface UserCompactionThresholdSettingsProps {
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	modelConfigsError?: unknown;
	isLoadingModelConfigs?: boolean;
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

export const UserCompactionThresholdSettings: FC<
	UserCompactionThresholdSettingsProps
> = ({ modelConfigs, modelConfigsError, isLoadingModelConfigs }) => {
	const queryClient = useQueryClient();
	const thresholdsQuery = useQuery(userCompactionThresholds());
	const [drafts, setDrafts] = useState<Record<string, string>>({});
	const [rowErrors, setRowErrors] = useState<Record<string, string>>({});
	const [pendingModels, setPendingModels] = useState<Set<string>>(new Set());

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

	const saveOpts = updateUserCompactionThreshold(queryClient);
	const saveThresholdMutation = useMutation({
		...saveOpts,
		onSuccess: async (_data, variables) => {
			await saveOpts.onSuccess?.();
			clearDraft(variables.modelConfigId);
			clearRowError(variables.modelConfigId);
		},
		onError: (error, variables) => {
			setRowErrors((currentErrors) => ({
				...currentErrors,
				[variables.modelConfigId]: getErrorMessage(
					error,
					"Failed to save compaction threshold.",
				),
			}));
		},
		onSettled: async (_data, _error, variables) => {
			setPendingModels((currentPendingModels) => {
				const nextPendingModels = new Set(currentPendingModels);
				nextPendingModels.delete(variables.modelConfigId);
				return nextPendingModels;
			});
		},
	});
	const resetOpts = deleteUserCompactionThreshold(queryClient);
	const resetThresholdMutation = useMutation({
		...resetOpts,
		onSuccess: async (_data, variables) => {
			await resetOpts.onSuccess?.();
			clearDraft(variables);
			clearRowError(variables);
		},
		onError: (error, variables) => {
			setRowErrors((currentErrors) => ({
				...currentErrors,
				[variables]: getErrorMessage(
					error,
					"Failed to reset compaction threshold.",
				),
			}));
		},
		onSettled: async (_data, _error, variables) => {
			setPendingModels((currentPendingModels) => {
				const nextPendingModels = new Set(currentPendingModels);
				nextPendingModels.delete(variables);
				return nextPendingModels;
			});
		},
	});

	const enabledModelConfigs = modelConfigs.filter((config) => config.enabled);
	const overridesByModelID = new Map(
		(thresholdsQuery.data?.thresholds ?? []).map(
			(threshold: TypesGen.UserChatCompactionThreshold) => [
				threshold.model_config_id,
				threshold.threshold_percent,
			],
		),
	);
	if (thresholdsQuery.isLoading) {
		return (
			<div className="space-y-2">
				<h3 className="m-0 text-[13px] font-semibold text-content-primary">
					Context Compaction
				</h3>
				<p className="!mt-0.5 m-0 text-xs text-content-secondary">
					Control when chat context is automatically summarized for each model.
					Setting 100% means the chat will never auto-compact.
				</p>
				<div className="flex items-center gap-2 text-sm text-content-secondary">
					<Spinner loading className="h-4 w-4" />
					Loading thresholds...
				</div>
			</div>
		);
	}

	if (thresholdsQuery.isError) {
		return (
			<div className="space-y-2">
				<h3 className="m-0 text-[13px] font-semibold text-content-primary">
					Context Compaction
				</h3>
				<p className="!mt-0.5 m-0 text-xs text-content-secondary">
					Control when chat context is automatically summarized for each model.
					Setting 100% means the chat will never auto-compact.
				</p>
				<p className="m-0 text-xs text-content-destructive">
					{getErrorMessage(
						thresholdsQuery.error,
						"Failed to load compaction thresholds.",
					)}
				</p>
			</div>
		);
	}

	return (
		<div className="space-y-2">
			<h3 className="m-0 text-[13px] font-semibold text-content-primary">
				Context Compaction
			</h3>
			<p className="!mt-0.5 m-0 text-xs text-content-secondary">
				Control when chat context is automatically summarized for each model.
				Setting 100% means the chat will never auto-compact.
			</p>
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
				<div className="space-y-2">
					{enabledModelConfigs.map((modelConfig) => {
						const existingOverride = overridesByModelID.get(modelConfig.id);
						const hasOverride = overridesByModelID.has(modelConfig.id);
						const draftValue =
							drafts[modelConfig.id] ??
							(existingOverride !== undefined ? String(existingOverride) : "");
						const parsedDraftValue = parseThresholdDraft(draftValue);
						const isThisModelMutating = pendingModels.has(modelConfig.id);
						const isSaveDisabled =
							draftValue.length === 0 ||
							parsedDraftValue === null ||
							parsedDraftValue === existingOverride ||
							isThisModelMutating;

						return (
							<div key={modelConfig.id} className="space-y-1">
								<div className="flex items-center justify-between gap-3">
									<div className="flex min-w-0 flex-1 items-baseline gap-2">
										<span className="text-[13px] font-medium text-content-primary">
											{modelConfig.display_name || modelConfig.model}
										</span>
										<span className="ml-auto text-xs text-content-secondary">
											System default:{" "}
											<span className="inline-block w-[4ch] text-right tabular-nums">
												{modelConfig.compression_threshold}%
											</span>
										</span>
									</div>
									<div className="flex items-center gap-1.5">
										<Input
											aria-label={`${modelConfig.display_name || modelConfig.model} compaction threshold`}
											type="number"
											min={0}
											max={100}
											inputMode="numeric"
											className="h-7 w-16 px-2 text-xs"
											value={draftValue}
											placeholder={String(modelConfig.compression_threshold)}
											onChange={(event) => {
												setDrafts((currentDrafts) => ({
													...currentDrafts,
													[modelConfig.id]: event.target.value,
												}));
												clearRowError(modelConfig.id);
											}}
											disabled={isThisModelMutating}
										/>
										<span className="text-xs text-content-secondary">%</span>
										<Button
											size="sm"
											className="h-7"
											type="button"
											disabled={isSaveDisabled}
											onClick={() => {
												if (parsedDraftValue === null) {
													return;
												}
												clearRowError(modelConfig.id);
												setPendingModels((currentPendingModels) =>
													new Set(currentPendingModels).add(modelConfig.id),
												);
												saveThresholdMutation.mutate({
													modelConfigId: modelConfig.id,
													req: {
														threshold_percent: parsedDraftValue,
													},
												});
											}}
										>
											Save
										</Button>
										{hasOverride && (
											<Button
												size="sm"
												className="h-7"
												variant="outline"
												type="button"
												disabled={isThisModelMutating}
												onClick={() => {
													clearRowError(modelConfig.id);
													setPendingModels((currentPendingModels) =>
														new Set(currentPendingModels).add(modelConfig.id),
													);
													resetThresholdMutation.mutate(modelConfig.id);
												}}
											>
												Reset
											</Button>
										)}
									</div>
								</div>
								{draftValue.length > 0 && parsedDraftValue === null && (
									<p className="m-0 text-xs text-content-destructive">
										Enter a whole number between 0 and 100.
									</p>
								)}
								{rowErrors[modelConfig.id] && (
									<p
										aria-live="polite"
										className="m-0 text-xs text-content-destructive"
									>
										{rowErrors[modelConfig.id]}
									</p>
								)}
								{draftValue === "100" && (
									<p className="m-0 text-xs text-content-secondary">
										⚠ Setting 100% will disable auto-compaction for this model.
									</p>
								)}
							</div>
						);
					})}
				</div>
			)}
		</div>
	);
};
