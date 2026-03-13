import { useTheme } from "@emotion/react";
import { getExternalImageStylesFromUrl } from "theme/externalImages";

export type ExternalImageProps = Omit<
	React.ComponentPropsWithRef<"img">,
	"alt"
> & {
	alt: string;
};

export const ExternalImage: React.FC<ExternalImageProps> = ({
	alt,
	...props
}) => {
	const theme = useTheme();

	return (
		<img
			css={getExternalImageStylesFromUrl(theme.externalImages, props.src)}
			{...props}
			alt={alt}
		/>
	);
};
