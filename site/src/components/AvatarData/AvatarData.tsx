import { useTheme } from "@emotion/react";
import type { FC, ReactNode } from "react";
import { Avatar } from "components/Avatar/Avatar";
import { Stack } from "components/Stack/Stack";

export interface AvatarDataProps {
  title: ReactNode;
  subtitle?: ReactNode;
  src?: string;
  avatar?: React.ReactNode;
}

export const AvatarData: FC<AvatarDataProps> = ({
  title,
  subtitle,
  src,
  avatar,
}) => {
  const theme = useTheme();

  if (!avatar) {
    avatar = <Avatar src={src}>{title}</Avatar>;
  }

  return (
    <Stack
      spacing={1.5}
      direction="row"
      alignItems="center"
      css={{
        minHeight: 40, // Make it predictable for the skeleton
        width: "100%",
        lineHeight: "150%",
      }}
    >
      {avatar}

      <Stack
        spacing={0}
        css={{
          width: "100%",
        }}
      >
        <span
          css={{
            color: theme.palette.text.primary,
            fontWeight: 600,
          }}
        >
          {title}
        </span>
        {subtitle && (
          <span
            css={{
              fontSize: 13,
              color: theme.palette.text.secondary,
              lineHeight: "150%",
              maxWidth: 540,
            }}
          >
            {subtitle}
          </span>
        )}
      </Stack>
    </Stack>
  );
};
