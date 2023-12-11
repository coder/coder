import { type ComponentProps, type FC } from "react";
import { useTheme } from "@emotion/react";
import RefreshIcon from "@mui/icons-material/RefreshOutlined";
import {
  HelpPopover,
  HelpTooltipAction,
  HelpTooltipContext,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import type { WorkspaceAgent } from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import { agentVersionStatus } from "../../utils/workspace";

type AgentOutdatedTooltipProps = ComponentProps<typeof HelpPopover> & {
  agent: WorkspaceAgent;
  serverVersion: string;
  status: agentVersionStatus;
  onUpdate: () => void;
};

export const AgentOutdatedTooltip: FC<AgentOutdatedTooltipProps> = ({
  agent,
  serverVersion,
  status,
  onUpdate,
  onOpen,
  id,
  open,
  onClose,
  anchorEl,
}) => {
  const theme = useTheme();
  const versionLabelStyles = {
    fontWeight: 600,
    color: theme.palette.text.primary,
  };
  const title =
    status === agentVersionStatus.Outdated
      ? "Agent Outdated"
      : "Agent Deprecated";
  const opener =
    status === agentVersionStatus.Outdated
      ? "This agent is an older version than the Coder server."
      : "This agent is using a deprecated version of the API.";
  const text =
    opener +
    " This can happen after you update Coder with running workspaces. " +
    "To fix this, you can stop and start the workspace.";

  return (
    <HelpPopover
      id={id}
      open={open}
      anchorEl={anchorEl}
      onOpen={onOpen}
      onClose={onClose}
    >
      <HelpTooltipContext.Provider value={{ open, onClose }}>
        <Stack spacing={1}>
          <div>
            <HelpTooltipTitle>{title}</HelpTooltipTitle>
            <HelpTooltipText>{text}</HelpTooltipText>
          </div>

          <Stack spacing={0.5}>
            <span css={versionLabelStyles}>Agent version</span>
            <span>{agent.version}</span>
          </Stack>

          <Stack spacing={0.5}>
            <span css={versionLabelStyles}>Server version</span>
            <span>{serverVersion}</span>
          </Stack>

          <HelpTooltipLinksGroup>
            <HelpTooltipAction
              icon={RefreshIcon}
              onClick={onUpdate}
              ariaLabel="Update workspace"
            >
              Update workspace
            </HelpTooltipAction>
          </HelpTooltipLinksGroup>
        </Stack>
      </HelpTooltipContext.Provider>
    </HelpPopover>
  );
};
