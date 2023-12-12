import { Stack } from "components/Stack/Stack";
import { type FC, type PropsWithChildren } from "react";

export const AppPreview: FC<PropsWithChildren> = ({ children }) => {
  return (
    <Stack
      css={(theme) => ({
        padding: "2px 12px",
        borderRadius: 9999,
        border: `1px solid ${theme.palette.divider}`,
        color: theme.palette.text.primary,
        background: theme.palette.background.paper,
        flexShrink: 0,
        width: "fit-content",
        fontSize: 12,

        "& img, & svg": {
          width: 13,
        },
      })}
      alignItems="center"
      direction="row"
      spacing={1}
    >
      {children}
    </Stack>
  );
};
