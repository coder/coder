import RetryIcon from "@mui/icons-material/CachedOutlined";
import ButtonGroup from "@mui/material/ButtonGroup";
import type { FC } from "react";
import type { Workspace } from "api/typesGenerated";
import { TopbarButton } from "components/FullPageLayout/Topbar";
import { BuildParametersPopover } from "./BuildParametersPopover";
import type { ActionButtonProps } from "./Buttons";

type RetryButtonProps = Omit<ActionButtonProps, "loading"> & {
  enableBuildParameters: boolean;
  workspace: Workspace;
};

export const RetryButton: FC<RetryButtonProps> = ({
  handleAction,
  workspace,
  enableBuildParameters,
}) => {
  const mainAction = (
    <TopbarButton startIcon={<RetryIcon />} onClick={() => handleAction()}>
      Retry
    </TopbarButton>
  );

  if (!enableBuildParameters) {
    return mainAction;
  }

  return (
    <ButtonGroup
      variant="outlined"
      css={{
        // Workaround to make the border transitions smoothly on button groups
        "& > button:hover + button": {
          borderLeft: "1px solid #FFF",
        },
      }}
    >
      {mainAction}
      <BuildParametersPopover
        label="Retry with build parameters"
        workspace={workspace}
        onSubmit={handleAction}
      />
    </ButtonGroup>
  );
};
