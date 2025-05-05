import { useTheme } from "@emotion/react";
import ComputerIcon from "@mui/icons-material/Computer";
import type { WorkspaceApp } from "api/typesGenerated";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import type { FC } from "react";
import { forDarkThemes, forLightThemes } from "theme/externalImages";

interface BaseIconProps {
	app: WorkspaceApp;
	onIconPathError?: () => void;
}

export const BaseIcon: FC<BaseIconProps> = ({ app, onIconPathError }) => {
	const theme = useTheme();

	return app.icon ? (
		<ExternalImage
			mode={theme.palette.mode === "dark" ? forLightThemes : forDarkThemes}
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
