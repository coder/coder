import { type ReactNode } from "react";
import { Avatar, AvatarIcon } from "components/Avatar/Avatar";
import { type CSSObject, useTheme } from "@emotion/react";

type AvatarCardProps = {
  header: ReactNode;
  imgUrl: string;
  altText: string;

  subtitle?: ReactNode;
};

export function AvatarCard({
  header,
  imgUrl,
  altText,
  subtitle,
}: AvatarCardProps) {
  const theme = useTheme();

  return (
    <article
      css={{
        width: "100%",
        display: "flex",
        flexFlow: "row nowrap",
        alignItems: "center",
        border: `1px solid ${theme.palette.divider}`,
        gap: "16px",
        padding: "16px",
        borderRadius: "8px",
        cursor: "default",
      }}
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
    </article>
  );
}
