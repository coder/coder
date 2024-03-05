import DebugIcon from "@mui/icons-material/BugReportOutlined";
import ButtonGroup from "@mui/material/ButtonGroup";
import type { FC } from "react";
import type { Workspace } from "api/typesGenerated";
import { TopbarButton } from "components/FullPageLayout/Topbar";
import { BuildParametersPopover } from "./BuildParametersPopover";
import type { ActionButtonProps } from "./Buttons";

type DebugButtonProps = Omit<ActionButtonProps, "loading"> & {
  workspace: Workspace;
  enableBuildParameters: boolean;
};

export const DebugButton: FC<DebugButtonProps> = ({
  handleAction,
  workspace,
  enableBuildParameters,
}) => {
  const mainAction = (
    <TopbarButton startIcon={<DebugIcon />} onClick={() => handleAction()}>
      Debug
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
        label="Debug with build parameters"
        workspace={workspace}
        onSubmit={handleAction}
      />
    </ButtonGroup>
  );
};
