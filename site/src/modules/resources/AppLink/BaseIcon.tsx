import { LaptopIcon } from "lucide-react";
import type { FC } from "react";
import type { WorkspaceApp } from "#/api/typesGenerated";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";

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
		<LaptopIcon />
	);
};
