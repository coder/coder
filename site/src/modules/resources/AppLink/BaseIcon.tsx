import type { WorkspaceApp } from "api/typesGenerated";
import {
	ExternalImage,
	type ExternalImageProps,
} from "components/ExternalImage";
import { LaptopIcon } from "lucide-react";
import type { FC } from "react";

interface BaseIconProps {
	app: WorkspaceApp;
	onIconPathError?: () => void;
}

export const BaseIcon: FC<BaseIconProps> = ({ app, onIconPathError }) => {
	if (!app.icon) {
		return <LaptopIcon />;
	}

	const imageProps: ExternalImageProps = {
		alt: `${app.display_name} icon`,
		src: app.icon,
		style: { pointerEvents: "none" },
		onError: () => {
			console.warn(
				`Application icon for "${app.id}" has invalid source "${app.icon}".`,
			);
			onIconPathError?.();
		},
	};

	return <ExternalImage {...imageProps} />;
};
