import { type FC } from "react";
import {
  containerWidth,
  containerWidthMedium,
  sidePadding,
} from "theme/constants";

type Size = "regular" | "medium" | "small";

const widthBySize: Record<Size, number> = {
  regular: containerWidth,
  medium: containerWidthMedium,
  small: containerWidth / 3,
};

export const Margins: FC<JSX.IntrinsicElements["div"] & { size?: Size }> = ({
  size = "regular",
  ...divProps
}) => {
  const maxWidth = widthBySize[size];
  return (
    <div
      {...divProps}
      css={{
        margin: "0 auto",
        maxWidth: maxWidth,
        padding: `0 ${sidePadding}px`,
        width: "100%",
      }}
    />
  );
};
