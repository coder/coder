import { useFormik } from "formik";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Alert, AlertDescription } from "#/components/Alert/Alert";
import { Button } from "#/components/Button/Button";
import { AdminBadge } from "./AdminBadge";
import type { ModelSelectorOption } from "./ChatElements/ModelSelector";
import { ModelSelector } from "./ChatElements/ModelSelector";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface ExploreModelOverrideSettingsProps {
	exploreModelOverrideData:
		| TypesGen.ChatExploreModelOverrideResponse
		| undefined;
	modelConfigs: readonly TypesGen.ChatModelConfig[];
	modelConfigsError: unknown;
	isLoadingModelConfigs: boolean;
	onSaveExploreModelOverride: (
		req: TypesGen.UpdateChatExploreModelOverrideRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingExploreModelOverride: boolean;
	isSaveExploreModelOverrideError: boolean;
	showHeader?: boolean;
}

const toModelSelectorOption = (
	modelConfig: TypesGen.ChatModelConfig,
): ModelSelectorOption => ({
	id: modelConfig.id,
	provider: modelConfig.provider,
	model: modelConfig.model,
	displayName: modelConfig.display_name.trim() || modelConfig.model,
	contextLimit: modelConfig.context_limit,
});

export const ExploreModelOverrideSettings: FC<
	ExploreModelOverrideSettingsProps
> = ({
	exploreModelOverrideData,
	modelConfigs,
	modelConfigsError,
	isLoadingModelConfigs,
	onSaveExploreModelOverride,
	isSavingExploreModelOverride,
	isSaveExploreModelOverrideError,
	showHeader = true,
}) => {
	const hasLoadedExploreModelOverride = exploreModelOverrideData !== undefined;
	const enabledModelOptions = modelConfigs
		.filter((modelConfig) => modelConfig.enabled)
		.map(toModelSelectorOption);

	const form = useFormik({
		enableReinitialize: true,
		initialValues: {
			model_config_id: exploreModelOverrideData?.model_config_id ?? "",
		},
		onSubmit: (values, { resetForm }) => {
			onSaveExploreModelOverride(
				{
					model_config_id: values.model_config_id || undefined,
				},
				{
					onSuccess: () => {
						resetForm({ values });
					},
				},
			);
		},
	});

	const isUnavailableSavedModel =
		form.values.model_config_id !== "" &&
		!enabledModelOptions.some(
			(option) => option.id === form.values.model_config_id,
		);
	const hasMalformedOverride =
		exploreModelOverrideData?.has_malformed_override ?? false;
	const isExploreModelOverrideDisabled =
		isSavingExploreModelOverride ||
		isLoadingModelConfigs ||
		!hasLoadedExploreModelOverride;
	const canSaveExploreModelOverride =
		hasLoadedExploreModelOverride && (form.dirty || hasMalformedOverride);

	return (
		<form className="space-y-2" onSubmit={form.handleSubmit}>
			{showHeader && (
				<>
					<div className="flex items-center gap-2">
						<h3 className="m-0 text-[13px] font-semibold text-content-primary">
							Explore subagent model
						</h3>
						<AdminBadge />
					</div>
					<p className="!mt-0.5 m-0 text-xs text-content-secondary">
						Optional deployment-wide model override for read-only Explore
						subagents spawned with <code>spawn_agent</code> using
						<code>type=explore</code>.
					</p>
				</>
			)}
			<div className="rounded-lg border border-border bg-surface-primary px-3 py-2">
				<ModelSelector
					options={enabledModelOptions}
					value={form.values.model_config_id}
					onValueChange={(value) =>
						form.setFieldValue("model_config_id", value)
					}
					disabled={isExploreModelOverrideDisabled}
					placeholder={
						isUnavailableSavedModel ? "Unavailable model" : "Use chat default"
					}
					emptyMessage={
						isLoadingModelConfigs
							? "Loading models..."
							: "No enabled models found."
					}
					className="h-10 w-full justify-between rounded-md border border-border border-solid bg-transparent px-3 text-sm shadow-sm"
					contentClassName="min-w-[18rem]"
				/>
			</div>
			{isUnavailableSavedModel && (
				<Alert severity="warning">
					<AlertDescription>
						The saved model is no longer enabled and will be ignored until you
						choose a new override.
					</AlertDescription>
				</Alert>
			)}
			{hasMalformedOverride && (
				<Alert severity="warning">
					<AlertDescription>
						The saved override is malformed and is being treated as unset. Click
						Save to clear it.
					</AlertDescription>
				</Alert>
			)}
			{Boolean(modelConfigsError) && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to load model configs.
				</p>
			)}
			<div className="flex justify-end gap-2">
				<Button
					size="sm"
					variant="outline"
					type="button"
					onClick={() => {
						form.setFieldValue("model_config_id", "");
					}}
					disabled={isExploreModelOverrideDisabled}
				>
					Clear
				</Button>
				<Button
					size="sm"
					type="submit"
					disabled={
						isExploreModelOverrideDisabled || !canSaveExploreModelOverride
					}
				>
					Save
				</Button>
			</div>
			{isSaveExploreModelOverrideError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save Explore model override.
				</p>
			)}
		</form>
	);
};
