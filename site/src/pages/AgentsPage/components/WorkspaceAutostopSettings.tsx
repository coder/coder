import { useFormik } from "formik";
import type { FC } from "react";
import { useState } from "react";
import * as Yup from "yup";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import { DurationField } from "./DurationField/DurationField";
import {
	TemporarySavedState,
	useTemporarySavedState,
} from "./TemporarySavedState";

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

const maxTTLMs = 30 * 24 * 60 * 60_000; // 30 days

export const WorkspaceAutostopSettings: FC<WorkspaceAutostopSettingsProps> = ({
	workspaceTTLData,
	isWorkspaceTTLLoading,
	isWorkspaceTTLLoadError,
	onSaveWorkspaceTTL,
	isSavingWorkspaceTTL,
	isSaveWorkspaceTTLError,
}) => {
	// ── Toggle state (fires immediate mutations, not a form submit) ──
	const [autostopToggled, setAutostopToggled] = useState<boolean | null>(null);
	const { isSavedVisible, showSavedState } = useTemporarySavedState();

	// ── Derived state ──
	const serverTTLMs = workspaceTTLData?.workspace_ttl_ms ?? 0;
	const isAutostopEnabled = autostopToggled ?? serverTTLMs > 0;

	// ── Form (for editing the TTL value) ──
	const validationSchema = Yup.object({
		workspace_ttl_ms: Yup.number()
			.required()
			.when([], {
				is: () => isAutostopEnabled,
				then: (schema) =>
					schema.moreThan(0, "Duration must be greater than zero."),
			})
			.max(maxTTLMs, "Must not exceed 30 days (720 hours)."),
	});

	const form = useFormik({
		initialValues: { workspace_ttl_ms: serverTTLMs },
		enableReinitialize: true,
		validationSchema,
		onSubmit: (values, helpers) => {
			onSaveWorkspaceTTL(
				{ workspace_ttl_ms: values.workspace_ttl_ms },
				{
					onSuccess: () => {
						showSavedState();
						setAutostopToggled(null);
						helpers.resetForm();
					},
					onError: () => setAutostopToggled(null),
				},
			);
		},
	});

	// ── Handlers ──
	const resetAutostopState = () => {
		setAutostopToggled(null);
		form.resetForm();
	};

	const handleToggleAutostop = (checked: boolean) => {
		if (checked) {
			// Defensive: restore server value if query cache is
			// stale; otherwise default to 1 hour.
			const defaultTTL = serverTTLMs > 0 ? serverTTLMs : 3_600_000;
			setAutostopToggled(true);
			void form.setFieldValue("workspace_ttl_ms", defaultTTL);
			onSaveWorkspaceTTL(
				{ workspace_ttl_ms: defaultTTL },
				{ onSuccess: resetAutostopState, onError: resetAutostopState },
			);
		} else {
			setAutostopToggled(false);
			void form.setFieldValue("workspace_ttl_ms", 0);
			onSaveWorkspaceTTL(
				{ workspace_ttl_ms: 0 },
				{ onSuccess: resetAutostopState, onError: resetAutostopState },
			);
		}
	};

	const handleTTLChange = (value: number) => {
		void form.setFieldValue("workspace_ttl_ms", value);
		// Latch the toggle open while the user is editing
		// so a background refetch cannot unmount the field.
		if (autostopToggled === null) {
			setAutostopToggled(true);
		}
	};

	const fieldError = form.errors.workspace_ttl_ms;

	return (
		<form className="flex flex-col gap-2" onSubmit={form.handleSubmit}>
			<div className="flex items-center justify-between gap-4">
				<div className="flex items-center gap-2">
					<h3 className="m-0 text-sm font-semibold text-content-primary">
						Workspace Autostop Fallback
					</h3>
				</div>
				<Switch
					checked={isAutostopEnabled}
					onCheckedChange={handleToggleAutostop}
					aria-label="Enable default autostop"
					disabled={isSavingWorkspaceTTL || isWorkspaceTTLLoading}
				/>
			</div>
			<p className="!mt-0.5 m-0 flex-1 text-xs text-content-secondary">
				Set a default autostop for agent-created workspaces that don't have one
				defined in their template. Template-defined autostop rules always take
				precedence. Active conversations will extend the stop time.
			</p>
			{isAutostopEnabled && (
				<DurationField
					valueMs={form.values.workspace_ttl_ms}
					onChange={handleTTLChange}
					label="Autostop Fallback"
					disabled={isSavingWorkspaceTTL || isWorkspaceTTLLoading}
					error={Boolean(fieldError)}
					helperText={fieldError}
				/>
			)}
			{isAutostopEnabled && (
				<div className="mt-2 flex min-h-6 justify-end">
					{(form.dirty || isSavedVisible || isSavingWorkspaceTTL) &&
						(isSavedVisible ? (
							<TemporarySavedState />
						) : (
							<Button
								size="xs"
								type="submit"
								disabled={
									isSavingWorkspaceTTL || !form.dirty || Boolean(fieldError)
								}
							>
								{isSavingWorkspaceTTL && (
									<Spinner loading className="h-4 w-4" />
								)}
								Save
							</Button>
						))}
				</div>
			)}
			{isSaveWorkspaceTTLError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save autostop setting.
				</p>
			)}
			{isWorkspaceTTLLoadError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to load autostop setting.
				</p>
			)}
		</form>
	);
};
