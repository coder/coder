import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { AdminBadge } from "./components/AdminBadge";
import { SectionHeader } from "./components/SectionHeader";
import { VirtualDesktopSettings } from "./components/VirtualDesktopSettings";
import type { MutationCallbacks } from "./types";

export interface AgentSettingsExperimentsPageViewProps {
	desktopEnabledData: TypesGen.ChatDesktopEnabledResponse | undefined;
	onSaveDesktopEnabled: (
		req: TypesGen.UpdateChatDesktopEnabledRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingDesktopEnabled: boolean;
	isSaveDesktopEnabledError: boolean;
}

export const AgentSettingsExperimentsPageView: FC<
	AgentSettingsExperimentsPageViewProps
> = ({
	desktopEnabledData,
	onSaveDesktopEnabled,
	isSavingDesktopEnabled,
	isSaveDesktopEnabledError,
}) => {
	return (
		<div className="flex flex-col gap-8">
			<SectionHeader
				label="Experiments"
				description="Opt in to experimental features."
				badge={<AdminBadge />}
			/>
			<VirtualDesktopSettings
				desktopEnabledData={desktopEnabledData}
				onSaveDesktopEnabled={onSaveDesktopEnabled}
				isSavingDesktopEnabled={isSavingDesktopEnabled}
				isSaveDesktopEnabledError={isSaveDesktopEnabledError}
			/>
		</div>
	);
};
