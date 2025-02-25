import ComputerIcon from "@mui/icons-material/Computer";
import type { WorkspaceApp } from "api/typesGenerated";
import type { FC } from "react";

interface BaseIconProps {
	app: WorkspaceApp;
	onIconPathError?: () => void;
}

export const BaseIcon: FC<BaseIconProps> = ({ app, onIconPathError }) => {
	return app.icon ? (
		<img
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
