import { useTheme } from "@emotion/react";
import { type ImgHTMLAttributes, forwardRef } from "react";
import {
	type ExternalImageModeStyles,
	getExternalImageStylesFromUrl,
} from "theme/externalImages";

type ExternalImageProps = ImgHTMLAttributes<HTMLImageElement> & {
	mode?: ExternalImageModeStyles;
};

export const ExternalImage = forwardRef<HTMLImageElement, ExternalImageProps>(
	({ mode, ...imgProps }, ref) => {
		const theme = useTheme();

		return (
			// biome-ignore lint/a11y/useAltText: alt should be passed in as a prop
			<img
				ref={ref}
				css={getExternalImageStylesFromUrl(
					mode ?? theme.externalImages,
					imgProps.src,
				)}
				{...imgProps}
			/>
		);
	},
);
