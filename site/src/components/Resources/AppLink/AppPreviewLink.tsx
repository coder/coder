import { Stack } from "components/Stack/Stack";
import { type FC } from "react";
import type * as TypesGen from "api/typesGenerated";
import { BaseIcon } from "./BaseIcon";
import { ShareIcon } from "./ShareIcon";

interface AppPreviewProps {
  app: TypesGen.WorkspaceApp;
}

export const AppPreviewLink: FC<AppPreviewProps> = ({ app }) => {
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
      <BaseIcon app={app} />
      {app.display_name}
      <ShareIcon app={app} />
    </Stack>
  );
};
