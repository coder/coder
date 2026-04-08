import type { FC, FormEvent } from "react";
import { useState } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Switch } from "#/components/Switch/Switch";
import { AdminBadge } from "./AdminBadge";
import { DurationField } from "./DurationField/DurationField";

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

export const WorkspaceAutostopSettings: FC<WorkspaceAutostopSettingsProps> = ({
	workspaceTTLData,
	isWorkspaceTTLLoading,
	isWorkspaceTTLLoadError,
	onSaveWorkspaceTTL,
	isSavingWorkspaceTTL,
	isSaveWorkspaceTTLError,
}) => {
	// ── Local state ──
	const [localTTLMs, setLocalTTLMs] = useState<number | null>(null);
	const [autostopToggled, setAutostopToggled] = useState<boolean | null>(null);

	// ── Derived state ──
	const serverTTLMs = workspaceTTLData?.workspace_ttl_ms ?? 0;
	const ttlMs = localTTLMs ?? serverTTLMs;
	const isAutostopEnabled = autostopToggled ?? serverTTLMs > 0;
	const isTTLDirty = localTTLMs !== null && localTTLMs !== serverTTLMs;
	const maxTTLMs = 30 * 24 * 60 * 60_000; // 30 days
	const isTTLOverMax = ttlMs > maxTTLMs;
	const isTTLZero = isAutostopEnabled && ttlMs === 0;

	// ── Handlers ──
	const resetAutostopState = () => {
		setLocalTTLMs(null);
		setAutostopToggled(null);
	};

	const handleToggleAutostop = (checked: boolean) => {
		if (checked) {
			// Defensive: restore server value if query cache is
			// stale; otherwise default to 1 hour.
			const defaultTTL = serverTTLMs > 0 ? serverTTLMs : 3_600_000;
			setAutostopToggled(true);
			setLocalTTLMs(defaultTTL);
			onSaveWorkspaceTTL(
				{ workspace_ttl_ms: defaultTTL },
				{ onSuccess: resetAutostopState, onError: resetAutostopState },
			);
		} else {
			setAutostopToggled(false);
			setLocalTTLMs(0);
			onSaveWorkspaceTTL(
				{ workspace_ttl_ms: 0 },
				{ onSuccess: resetAutostopState, onError: resetAutostopState },
			);
		}
	};

	const handleSaveChatWorkspaceTTL = (event: FormEvent) => {
		event.preventDefault();
		if (!isTTLDirty || isSavingWorkspaceTTL) return;
		onSaveWorkspaceTTL(
			{ workspace_ttl_ms: localTTLMs ?? 0 },
			{
				onSuccess: resetAutostopState,
				onError: () => setAutostopToggled(null),
			},
		);
	};

	const handleTTLChange = (value: number) => {
		setLocalTTLMs(value);
		// Latch the toggle open while the user is editing
		// so a background refetch cannot unmount the field.
		if (autostopToggled === null) {
			setAutostopToggled(true);
		}
	};

	return (
		<form
			className="space-y-2"
			onSubmit={(event) => void handleSaveChatWorkspaceTTL(event)}
		>
			<div className="flex items-center gap-2">
				<h3 className="m-0 text-[13px] font-semibold text-content-primary">
					Workspace Autostop Fallback
				</h3>
				<AdminBadge />
			</div>
			<div className="flex items-center justify-between gap-4">
				<p className="!mt-0.5 m-0 flex-1 text-xs text-content-secondary">
					Set a default autostop for agent-created workspaces that don't have
					one defined in their template. Template-defined autostop rules always
					take precedence. Active conversations will extend the stop time.
				</p>
				<Switch
					checked={isAutostopEnabled}
					onCheckedChange={handleToggleAutostop}
					aria-label="Enable default autostop"
					disabled={isSavingWorkspaceTTL || isWorkspaceTTLLoading}
				/>{" "}
			</div>
			{isAutostopEnabled && (
				<DurationField
					valueMs={ttlMs}
					onChange={handleTTLChange}
					label="Autostop Fallback"
					disabled={isSavingWorkspaceTTL || isWorkspaceTTLLoading}
					error={isTTLOverMax || isTTLZero}
					helperText={
						isTTLZero
							? "Duration must be greater than zero."
							: isTTLOverMax
								? "Must not exceed 30 days (720 hours)."
								: undefined
					}
				/>
			)}
			{isAutostopEnabled && (
				<div className="flex justify-end">
					<Button
						size="sm"
						type="submit"
						disabled={
							isSavingWorkspaceTTL || !isTTLDirty || isTTLOverMax || isTTLZero
						}
					>
						Save
					</Button>
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
