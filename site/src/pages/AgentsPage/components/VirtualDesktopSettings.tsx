import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Link } from "#/components/Link/Link";
import { Switch } from "#/components/Switch/Switch";

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
		<div className="flex flex-col gap-2">
			<div className="flex items-center justify-between gap-4">
				<div className="flex items-center gap-2">
					<h3 className="m-0 text-sm font-semibold text-content-primary">
						Virtual Desktop
					</h3>
					<Badge size="sm" variant="warning" className="cursor-default">
						<TriangleAlertIcon className="h-3 w-3" />
						Experimental feature
					</Badge>
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
			<div className="m-0 flex-1 text-xs text-content-secondary">
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
			</div>
			{isSaveDesktopEnabledError && (
				<p className="m-0 text-xs text-content-destructive">
					Failed to save desktop setting.
				</p>
			)}
		</div>
	);
};
