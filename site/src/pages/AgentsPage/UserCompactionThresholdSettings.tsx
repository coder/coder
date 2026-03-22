import { getErrorMessage } from "api/errors";
import {
	deleteUserCompactionThreshold,
	updateUserCompactionThreshold,
	userCompactionThresholds,
} from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Input } from "components/Input/Input";
import { Spinner } from "components/Spinner/Spinner";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";

interface UserCompactionThresholdSettingsProps {
	modelConfigs: readonly TypesGen.ChatModelConfig[];
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
> = ({ modelConfigs }) => {
	const queryClient = useQueryClient();
	const thresholdsQuery = useQuery(userCompactionThresholds());
	const saveThresholdMutation = useMutation(
		updateUserCompactionThreshold(queryClient),
	);
	const resetThresholdMutation = useMutation(
		deleteUserCompactionThreshold(queryClient),
	);
	const [drafts, setDrafts] = useState<Record<string, string>>({});
	const [rowErrors, setRowErrors] = useState<Record<string, string>>({});

	const enabledModelConfigs = modelConfigs.filter((config) => config.enabled);
	const overridesByModelID = new Map(
		(thresholdsQuery.data?.thresholds ?? []).map((threshold) => [
			threshold.model_config_id,
			threshold.threshold_percent,
		]),
	);
	const isMutating =
		saveThresholdMutation.isPending || resetThresholdMutation.isPending;

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
			{enabledModelConfigs.length === 0 ? (
				<p className="m-0 text-xs text-content-secondary">
					No enabled chat models available.
				</p>
			) : (
				<div className="space-y-3">
					{enabledModelConfigs.map((modelConfig) => {
						const existingOverride = overridesByModelID.get(modelConfig.id);
						const hasOverride = overridesByModelID.has(modelConfig.id);
						const draftValue =
							drafts[modelConfig.id] ??
							(existingOverride !== undefined ? String(existingOverride) : "");
						const parsedDraftValue = parseThresholdDraft(draftValue);
						const isSaveDisabled =
							draftValue.length === 0 ||
							parsedDraftValue === null ||
							parsedDraftValue === existingOverride ||
							isMutating;

						return (
							<div
								key={modelConfig.id}
								className="space-y-2 rounded-lg border border-border bg-surface-secondary/40 p-4"
							>
								<div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
									<div className="flex-1">
										<span className="text-sm font-medium text-content-primary">
											{modelConfig.display_name}
										</span>
										<span className="ml-2 text-xs text-content-secondary">
											Default: {modelConfig.compression_threshold}%
										</span>
									</div>
									<div className="flex flex-wrap items-center gap-2 sm:justify-end">
										<Input
											aria-label={`${modelConfig.display_name} compaction threshold`}
											type="number"
											min={0}
											max={100}
											inputMode="numeric"
											className="h-9 w-20 text-[13px]"
											value={draftValue}
											placeholder={String(modelConfig.compression_threshold)}
											onChange={(event) => {
												setDrafts((currentDrafts) => ({
													...currentDrafts,
													[modelConfig.id]: event.target.value,
												}));
												clearRowError(modelConfig.id);
											}}
											disabled={isMutating}
										/>
										<span className="text-xs text-content-secondary">%</span>
										<Button
											size="sm"
											type="button"
											disabled={isSaveDisabled}
											onClick={() => {
												if (parsedDraftValue === null) {
													return;
												}
												clearRowError(modelConfig.id);
												saveThresholdMutation.mutate(
													{
														modelConfigId: modelConfig.id,
														req: {
															threshold_percent: parsedDraftValue,
														},
													},
													{
														onSuccess: () => {
															clearDraft(modelConfig.id);
															clearRowError(modelConfig.id);
														},
														onError: (error) => {
															setRowErrors((currentErrors) => ({
																...currentErrors,
																[modelConfig.id]: getErrorMessage(
																	error,
																	"Failed to save compaction threshold.",
																),
															}));
														},
													},
												);
											}}
										>
											Save
										</Button>
										{hasOverride && (
											<Button
												size="sm"
												variant="outline"
												type="button"
												disabled={isMutating}
												onClick={() => {
													clearRowError(modelConfig.id);
													resetThresholdMutation.mutate(modelConfig.id, {
														onSuccess: () => {
															clearDraft(modelConfig.id);
															clearRowError(modelConfig.id);
														},
														onError: (error) => {
															setRowErrors((currentErrors) => ({
																...currentErrors,
																[modelConfig.id]: getErrorMessage(
																	error,
																	"Failed to reset compaction threshold.",
																),
															}));
														},
													});
												}}
											>
												Reset
											</Button>
										)}
									</div>
								</div>
								{rowErrors[modelConfig.id] && (
									<p className="m-0 text-xs text-content-destructive">
										{rowErrors[modelConfig.id]}
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
