import ComputerIcon from "@mui/icons-material/Computer";
import type { WorkspaceApp } from "api/typesGenerated";
import type { FC } from "react";

interface BaseIconProps {
	app: WorkspaceApp;
	onError?: () => void;
}

export const BaseIcon: FC<BaseIconProps> = ({ app, onError }) => {
	return app.icon ? (
		<img
			alt={`${app.display_name} Icon`}
			src={app.icon}
			style={{ pointerEvents: "none" }}
			onError={() => {
				onError?.();
			}}
		/>
	) : (
		<ComputerIcon />
	);
};
