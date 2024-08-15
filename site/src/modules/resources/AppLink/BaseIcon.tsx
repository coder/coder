import ComputerIcon from "@mui/icons-material/Computer";
import type { WorkspaceApp } from "api/typesGenerated";
import type { FC } from "react";

interface BaseIconProps {
	app: WorkspaceApp;
}

export const BaseIcon: FC<BaseIconProps> = ({ app }) => {
	return app.icon ? (
		<img
			alt={`${app.display_name} Icon`}
			src={app.icon}
			style={{ pointerEvents: "none" }}
		/>
	) : (
		<ComputerIcon />
	);
};
