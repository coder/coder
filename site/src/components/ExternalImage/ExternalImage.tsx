import { useTheme } from "@emotion/react";
import { type ImgHTMLAttributes, forwardRef } from "react";
import { getExternalImageStylesFromUrl } from "theme/externalImages";

export const ExternalImage = forwardRef<
	HTMLImageElement,
	ImgHTMLAttributes<HTMLImageElement>
>((attrs, ref) => {
	const theme = useTheme();

	return (
		// biome-ignore lint/a11y/useAltText: no reasonable alt to provide
		<img
			ref={ref}
			css={getExternalImageStylesFromUrl(theme.externalImages, attrs.src)}
			{...attrs}
		/>
	);
});
