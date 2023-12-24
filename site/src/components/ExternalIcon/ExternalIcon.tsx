import { type Interpolation, type Theme } from "@emotion/react";
import { type FC, type ImgHTMLAttributes } from "react";

interface ExternalIconProps extends ImgHTMLAttributes<HTMLImageElement> {
  size?: number;
}

export const ExternalIcon: FC<ExternalIconProps> = ({
  size = 36,
  ...attrs
}) => {
  return (
    <div css={[styles.container, { height: size, width: size }]}>
      <img
        alt=""
        aria-hidden
        css={[
          styles.icon,
          { height: size, width: size, padding: Math.ceil(size / 6) },
        ]}
        {...attrs}
      />
    </div>
  );
};

const styles = {
  container: {
    borderRadius: 9999,
    overflow: "clip",
  },
  icon: {
    backgroundColor: "#000",
    objectFit: "contain",
  },
} satisfies Record<string, Interpolation<Theme>>;
