import { HTMLAttributes, forwardRef } from "react";

type ImageIconProps = {
  size: string | number;
} & HTMLAttributes<HTMLDivElement>;

/**
 * A component designed to render an image icon, this code creates a container
 * (box) and ensures the centered placement of the image within it. The size of the
 * image is dynamically adjusted based on the 'size' prop, maintaining proportional
 * scaling—allowing for both enlargement and reduction.
 */
export const ImageIcon = forwardRef<HTMLDivElement, ImageIconProps>(
  (props, ref) => {
    const { size, ...divProps } = props;
    return (
      <div
        ref={ref}
        css={{
          width: size,
          height: size,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          flexShrink: 0,
          lineHeight: 0,

          "& img": {
            objectFit: "contain",
            width: "100%",
            height: "100%",
          },
        }}
        {...divProps}
      />
    );
  },
);
