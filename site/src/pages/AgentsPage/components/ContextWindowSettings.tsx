import { useFormik } from "formik";
import type * as TypesGen from "#/api/typesGenerated";
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

export type ContextWindowSettingsProps = {
	contextWindowConfigData: TypesGen.ChatContextWindowConfig | undefined;
	isLoadingContextWindowConfig: boolean;
	onSaveContextWindowConfig: (
		req: TypesGen.UpdateChatContextWindowConfigRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingContextWindowConfig: boolean;
	contextWindowConfigSaveError: Error | null;
};

const fallbackOptions = [
	{ label: "Summarize the conversation", value: "summary" },
	{ label: "Start a new context window", value: "new_window" },
] as const satisfies readonly {
	label: string;
	value: TypesGen.ChatContextWindowFallback;
}[];

const getFallbackLabel = (fallback: string) =>
	fallbackOptions.find((option) => option.value === fallback)?.label ??
	fallback;

/**
 * Deployment setting for the token-budget context window prototype. It
 * picks what happens when an agent reaches the end of its context window
 * without calling new_context.
 */
export const ContextWindowSettings: React.FC<ContextWindowSettingsProps> = ({
	contextWindowConfigData,
	isLoadingContextWindowConfig,
	onSaveContextWindowConfig,
	isSavingContextWindowConfig,
	contextWindowConfigSaveError,
}) => {
	const { isSavedVisible, showSavedState } = useTemporarySavedState();
	const hasLoaded = contextWindowConfigData !== undefined;

	const form = useFormik<{ fallback: TypesGen.ChatContextWindowFallback }>({
		enableReinitialize: true,
		initialValues: {
			fallback: contextWindowConfigData?.fallback ?? "summary",
		},
		onSubmit: (values, helpers) => {
			onSaveContextWindowConfig(
				{ fallback: values.fallback },
				{
					onSuccess: () => {
						showSavedState();
						helpers.resetForm({ values });
					},
				},
			);
		},
	});

	const isFormDisabled =
		isSavingContextWindowConfig || isLoadingContextWindowConfig || !hasLoaded;
	const canSave = hasLoaded && form.dirty;

	return (
		<AgentSettingLayout
			title="Context window fallback"
			description="Agents save notes and start a new context window when their context fills up. Choose what happens when an agent reaches the end of its window without starting a new one."
			showSave={canSave}
			isSaving={isSavingContextWindowConfig}
			isSavedVisible={isSavedVisible}
			saveDisabled={isFormDisabled || !canSave}
			onSubmit={form.handleSubmit}
			error={
				contextWindowConfigSaveError ? (
					<p className="m-0">Failed to save the context window fallback.</p>
				) : undefined
			}
		>
			<div className="flex w-88 max-w-full flex-col gap-2">
				<Select
					value={form.values.fallback}
					onValueChange={(value) => void form.setFieldValue("fallback", value)}
					disabled={isFormDisabled}
				>
					<SelectTrigger
						aria-label="Context window fallback"
						className="h-10 w-full justify-between rounded-md border border-border border-solid bg-transparent px-3 text-sm shadow-none"
					>
						<SelectValue placeholder="Select fallback">
							{isLoadingContextWindowConfig ? (
								<Skeleton className="h-4 w-20" aria-hidden="true" />
							) : (
								getFallbackLabel(form.values.fallback)
							)}
						</SelectValue>
					</SelectTrigger>
					<SelectContent align="end" className="min-w-44">
						<SelectGroup>
							{fallbackOptions.map((option) => (
								<SelectItem key={option.value} value={option.value}>
									{option.label}
								</SelectItem>
							))}
						</SelectGroup>
					</SelectContent>
				</Select>
			</div>
		</AgentSettingLayout>
	);
};
