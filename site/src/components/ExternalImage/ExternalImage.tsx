import { useTheme } from "@emotion/react";
import { getExternalImageStylesFromUrl } from "theme/externalImages";

export const ExternalImage: React.FC<React.ComponentPropsWithRef<"img">> = ({
	...props
}) => {
	const theme = useTheme();

	return (
		// biome-ignore lint/a11y/useAltText: alt should be passed in as a prop
		<img
			css={getExternalImageStylesFromUrl(theme.externalImages, props.src)}
			{...props}
		/>
	);
};
