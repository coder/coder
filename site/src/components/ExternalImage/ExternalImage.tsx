import { useTheme } from "@emotion/react";
import { getExternalImageStylesFromUrl } from "#/theme/externalImages";

export const ExternalImage: React.FC<React.ComponentPropsWithRef<"img">> = ({
	style,
	...props
}) => {
	const theme = useTheme();

	return (
		// oxlint-disable-next-line jsx-a11y/alt-text -- alt should be passed in as a prop
		<img
			style={{
				...getExternalImageStylesFromUrl(theme.externalImages, props.src),
				...style,
			}}
			{...props}
		/>
	);
};
