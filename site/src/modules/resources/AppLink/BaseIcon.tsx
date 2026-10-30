import { LaptopIcon } from "lucide-react";
import type { WorkspaceApp } from "#/api/typesGenerated";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";

type BaseIconProps = {
	app: WorkspaceApp;
	// Defaults to naming the app. Pass "" when a visible label already does.
	alt?: string;
	onIconPathError?: () => void;
};

export const BaseIcon: React.FC<BaseIconProps> = ({
	app,
	alt = `${app.display_name} Icon`,
	onIconPathError,
}) => {
	return app.icon ? (
		<ExternalImage
			alt={alt}
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
