import { useTheme } from "@emotion/react";
import { getExternalImageStylesFromUrl } from "#/theme/externalImages";

export const ExternalImage: React.FC<React.ComponentPropsWithRef<"img">> = ({
	style,
	alt = "",
	...props
}) => {
	const theme = useTheme();

	return (
		<img
			alt={alt}
			style={{
				...getExternalImageStylesFromUrl(theme.externalImages, props.src),
				...style,
			}}
			{...props}
		/>
	);
};
