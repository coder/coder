import { useFormik } from "formik";
import type { FC } from "react";
import * as Yup from "yup";
import type * as TypesGen from "#/api/typesGenerated";
import { useTemporarySavedState } from "#/components/TemporarySavedState/TemporarySavedState";
import { DurationField } from "./DurationField/DurationField";
import { LifecycleSettingLayout } from "./LifecycleSettingLayout";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface WorkspaceAutostopSettingsProps {
	workspaceTTLData: TypesGen.ChatWorkspaceTTLResponse | undefined;
	isWorkspaceTTLLoading: boolean;
	isWorkspaceTTLLoadError: boolean;
	onSaveWorkspaceTTL: (
		req: TypesGen.UpdateChatWorkspaceTTLRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingWorkspaceTTL: boolean;
	isSaveWorkspaceTTLError: boolean;
}

const DEFAULT_WORKSPACE_TTL_MS = 3_600_000;
const maxTTLMs = 30 * 24 * 60 * 60_000;

const validationSchema = Yup.object({
	enabled: Yup.boolean().required(),
	workspace_ttl_ms: Yup.number().when("enabled", {
		is: true,
		then: (schema) =>
			schema
				.required()
				.moreThan(0, "Duration must be greater than zero.")
				.max(maxTTLMs, "Must not exceed 30 days (720 hours)."),
	}),
});

export const WorkspaceAutostopSettings: FC<WorkspaceAutostopSettingsProps> = ({
	workspaceTTLData,
	isWorkspaceTTLLoading,
	isWorkspaceTTLLoadError,
	onSaveWorkspaceTTL,
	isSavingWorkspaceTTL,
	isSaveWorkspaceTTLError,
}) => {
	const { isSavedVisible, showSavedState } = useTemporarySavedState();
	const serverTTLMs = workspaceTTLData?.workspace_ttl_ms ?? 0;

	const form = useFormik({
		initialValues: {
			enabled: serverTTLMs > 0,
			workspace_ttl_ms:
				serverTTLMs > 0 ? serverTTLMs : DEFAULT_WORKSPACE_TTL_MS,
		},
		enableReinitialize: true,
		validationSchema,
		onSubmit: (values, helpers) => {
			onSaveWorkspaceTTL(
				{ workspace_ttl_ms: values.enabled ? values.workspace_ttl_ms : 0 },
				{
					onSuccess: () => {
						showSavedState();
						helpers.resetForm({ values });
					},
				},
			);
		},
	});

	const handleToggleAutostop = (checked: boolean) => {
		void form.setFieldValue("enabled", checked);
		if (checked && form.values.workspace_ttl_ms <= 0) {
			void form.setFieldValue("workspace_ttl_ms", DEFAULT_WORKSPACE_TTL_MS);
		}
	};

	const handleTTLChange = (value: number) => {
		void form.setFieldValue("workspace_ttl_ms", value);
	};

	const fieldError = form.errors.workspace_ttl_ms;
	const hasError =
		Boolean(fieldError) || isSaveWorkspaceTTLError || isWorkspaceTTLLoadError;

	return (
		<LifecycleSettingLayout
			title="Workspace autostop fallback"
			description="Set a default autostop for agent-created workspaces that don't have one defined in their template. Template-defined autostop rules always take precedence. Active conversations will extend the stop time."
			checked={form.values.enabled}
			onCheckedChange={handleToggleAutostop}
			switchLabel="Enable default autostop"
			disabled={isSavingWorkspaceTTL || isWorkspaceTTLLoading}
			showSave={form.dirty}
			isSaving={isSavingWorkspaceTTL}
			isSavedVisible={isSavedVisible}
			saveDisabled={isSavingWorkspaceTTL || !form.dirty || Boolean(fieldError)}
			onSubmit={form.handleSubmit}
			error={
				hasError ? (
					<>
						{/* DurationField manages its own text state and never calls
						   Formik's onBlur, so form.touched is never set for this
						   field. We display the error directly when present. */}
						{fieldError && <p className="m-0">{fieldError}</p>}
						{isSaveWorkspaceTTLError && (
							<p className="m-0">Failed to save autostop setting.</p>
						)}
						{isWorkspaceTTLLoadError && (
							<p className="m-0">Failed to load autostop setting.</p>
						)}
					</>
				) : undefined
			}
		>
			<DurationField
				valueMs={form.values.workspace_ttl_ms}
				onChange={handleTTLChange}
				label="Autostop fallback"
				disabled={
					!form.values.enabled || isSavingWorkspaceTTL || isWorkspaceTTLLoading
				}
				error={Boolean(fieldError)}
				className="w-fit"
			/>
		</LifecycleSettingLayout>
	);
};
