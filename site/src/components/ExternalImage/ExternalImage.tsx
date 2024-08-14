import { useTheme } from "@emotion/react";
import { type ImgHTMLAttributes, forwardRef } from "react";
import { getExternalImageStylesFromUrl } from "theme/externalImages";

export const ExternalImage = forwardRef<
  HTMLImageElement,
  ImgHTMLAttributes<HTMLImageElement>
>((attrs, ref) => {
  const theme = useTheme();

  return (
    <img
      ref={ref}
      alt=""
      css={getExternalImageStylesFromUrl(theme.externalImages, attrs.src)}
      {...attrs}
    />
  );
});
