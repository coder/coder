import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Link } from "#/components/Link/Link";
import { Switch } from "#/components/Switch/Switch";
import { AdminBadge } from "./AdminBadge";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface VirtualDesktopSettingsProps {
	desktopEnabledData: TypesGen.ChatDesktopEnabledResponse | undefined;
	onSaveDesktopEnabled: (
		req: TypesGen.UpdateChatDesktopEnabledRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingDesktopEnabled: boolean;
	isSaveDesktopEnabledError: boolean;
}

export const VirtualDesktopSettings: FC<VirtualDesktopSettingsProps> = ({
	desktopEnabledData,
	onSaveDesktopEnabled,
	isSavingDesktopEnabled,
	isSaveDesktopEnabledError,
}) => {
	const desktopEnabled = desktopEnabledData?.enable_desktop ?? false;

	return (
		<div className="space-y-2">
			<div className="flex items-center gap-2">
				<h3 className="m-0 text-[13px] font-semibold text-content-primary">
					Virtual Desktop
				</h3>
				<AdminBadge />
			</div>
			<div className="flex items-center justify-between gap-4">
				<div className="!mt-0.5 m-0 flex-1 text-xs text-content-secondary">
					<p className="m-0">
						Allow agents to use a virtual, graphical desktop within workspaces.
						Requires the{" "}
						<Link
							href="https://registry.coder.com/modules/coder/portabledesktop"
							target="_blank"
							size="sm"
						>
							portabledesktop module
						</Link>{" "}
						to be installed in the workspace and the Anthropic provider to be
						configured.
					</p>
					<p className="mt-2 mb-0 font-semibold text-content-secondary">
						Warning: This is a work-in-progress feature, and you're likely to
						encounter bugs if you enable it.
					</p>
				</div>
				<Switch
					checked={desktopEnabled}
					onCheckedChange={(checked) =>
						onSaveDesktopEnabled({ enable_desktop: checked })
					}
					aria-label="Enable"
					disabled={isSavingDesktopEnabled}
				/>
			</div>
			{isSaveDesktopEnabledError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save desktop setting.
				</p>
			)}
		</div>
	);
};
