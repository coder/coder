import type { FC } from "react";
import type { UseMutateFunction } from "react-query";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Switch } from "#/components/Switch/Switch";

interface PersonalMemorySettingsProps {
	/** Undefined while loading or when the experiment is off. */
	settings: TypesGen.ChatPersonalMemorySettings | undefined;
	/** Set when the settings request failed and no cached value exists. */
	loadError?: unknown;
	onRetryLoad?: () => void;
	onSaveSettings: UseMutateFunction<
		void,
		Error,
		TypesGen.UpdateChatPersonalMemorySettingsRequest,
		unknown
	>;
	isSavingSettings: boolean;
	isSaveSettingsError: boolean;
}

/**
 * Opt-out for personal memory, the only memory control in the UI. Memories
 * themselves are read and edited by the agent through its tools.
 */
export const PersonalMemorySettings: FC<PersonalMemorySettingsProps> = ({
	settings,
	loadError,
	onRetryLoad,
	onSaveSettings,
	isSavingSettings,
	isSaveSettingsError,
}) => {
	if (!settings && !loadError) {
		return null;
	}

	return (
		<div className="space-y-2">
			<h3 className="m-0 text-sm font-semibold text-content-primary">
				Save and use personal memory
			</h3>
			<div className="flex items-center justify-between gap-4">
				<p className="mt-0.5! m-0 flex-1 text-xs text-content-secondary">
					Let the agent remember how you like to work across chats that are not
					in a project. Turning this off stops new chats from reading or writing
					personal memory.
				</p>
				{settings ? (
					<Switch
						checked={settings.enabled}
						onCheckedChange={(checked) => onSaveSettings({ enabled: checked })}
						aria-label="Save and use personal memory"
						disabled={isSavingSettings}
					/>
				) : (
					<Button size="sm" variant="outline" onClick={onRetryLoad}>
						Retry
					</Button>
				)}
			</div>
			{loadError != null && !settings && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to load your personal memory preference.
				</p>
			)}
			{isSaveSettingsError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save your personal memory preference.
				</p>
			)}
		</div>
	);
};
