import { type ReactNode } from "react";
import { Avatar } from "components/Avatar/Avatar";
import { type CSSObject, useTheme } from "@emotion/react";

type AvatarCardProps = {
  header: string;
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
      <div css={{ marginRight: "auto", minWidth: 0 }}>
        <h3
          title={header}
          css={[
            theme.typography.body1 as CSSObject,
            {
              lineHeight: 1.5,
              margin: 0,
              overflow: "hidden",
              whiteSpace: "nowrap",
              textOverflow: "ellipsis",
            },
          ]}
        >
          {header}
        </h3>

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

      <Avatar src={imgUrl} alt={altText} colorScheme="darken">
        {header}
      </Avatar>
    </article>
  );
}
