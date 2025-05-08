import type { WorkspaceApp } from "api/typesGenerated";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { ComputerIcon } from "lucide-react";
import type { FC } from "react";

interface BaseIconProps {
	app: WorkspaceApp;
	onIconPathError?: () => void;
}

export const BaseIcon: FC<BaseIconProps> = ({ app, onIconPathError }) => {
	return app.icon ? (
		<ExternalImage
			alt={`${app.display_name} Icon`}
			src={app.icon}
			style={{ pointerEvents: "none" }}
			onError={() => {
				console.warn(
					`Application icon for "${app.id}" has invalid source "${app.icon}".`,
				);
				onIconPathError?.();
			}}
		/>
	) : (
		<ComputerIcon />
	);
};
