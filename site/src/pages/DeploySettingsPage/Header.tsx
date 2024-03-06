import { useTheme } from "@emotion/react";
import LaunchOutlined from "@mui/icons-material/LaunchOutlined";
import Button from "@mui/material/Button";
import type { FC, ReactNode } from "react";
import { Stack } from "components/Stack/Stack";

interface HeaderProps {
  title: ReactNode;
  description?: ReactNode;
  secondary?: boolean;
  docsHref?: string;
}

export const Header: FC<HeaderProps> = ({
  title,
  description,
  docsHref,
  secondary,
}) => {
  const theme = useTheme();

  return (
    <Stack alignItems="baseline" direction="row" justifyContent="space-between">
      <div css={{ maxWidth: 420, marginBottom: 24 }}>
        <h1
          css={[
            {
              fontSize: 32,
              fontWeight: 700,
              display: "flex",
              alignItems: "center",
              lineHeight: "initial",
              margin: 0,
              marginBottom: 4,
              gap: 8,
            },
            secondary && {
              fontSize: 24,
              fontWeight: 500,
            },
          ]}
        >
          {title}
        </h1>
        {description && (
          <span
            css={{
              fontSize: 14,
              color: theme.palette.text.secondary,
              lineHeight: "160%",
            }}
          >
            {description}
          </span>
        )}
      </div>

      {docsHref && (
        <Button
          startIcon={<LaunchOutlined />}
          component="a"
          href={docsHref}
          target="_blank"
        >
          Read the docs
        </Button>
      )}
    </Stack>
  );
};
