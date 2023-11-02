import { type ReactNode } from "react";
import { Avatar, AvatarIcon } from "components/Avatar/Avatar";
import {
  type CSSObject,
  type Interpolation,
  type Theme,
  useTheme,
} from "@emotion/react";

type Width = "sm" | "md" | "lg" | "full";

type AvatarCardProps = {
  header: ReactNode;
  imgUrl: string;
  altText: string;

  subtitle?: ReactNode;
  width?: Width;
};

const renderedWidths = {
  sm: (theme) => theme.spacing(2),
  md: (theme) => theme.spacing(3),
  lg: (theme) => theme.spacing(6),
  full: { width: "100%" },
} as const satisfies Record<Width, Interpolation<Theme>>;

export function AvatarCard({
  header,
  imgUrl,
  altText,
  subtitle,
  width = "full",
}: AvatarCardProps) {
  const theme = useTheme();

  return (
    <div
      css={[
        renderedWidths[width],
        {
          display: "flex",
          flexFlow: "row nowrap",
          alignItems: "center",
          border: `1px solid ${theme.palette.divider}`,
          gap: theme.spacing(2),
          padding: theme.spacing(2),
          borderRadius: theme.shape.borderRadius,
          cursor: "default",
        },
      ]}
    >
      <div css={{ marginRight: "auto" }}>
        <div css={theme.typography.body1 as CSSObject}>{header}</div>

        {subtitle && (
          <div
            css={[
              theme.typography.body2 as CSSObject,
              { color: theme.palette.text.secondary },
            ]}
          >
            {subtitle}
          </div>
        )}
      </div>

      <div>
        <Avatar>
          <AvatarIcon src={imgUrl} alt={altText} />
        </Avatar>
      </div>
    </div>
  );
}
