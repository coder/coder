import { type FC } from "react";
import { useTheme } from "@emotion/react";
import RefreshIcon from "@mui/icons-material/RefreshOutlined";
import {
  HelpTooltip,
  HelpTooltipAction,
  HelpTooltipContent,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/HelpTooltip/HelpTooltip";
import type { WorkspaceAgent } from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import { agentVersionStatus } from "../../utils/workspace";
import { PopoverTrigger } from "components/Popover/Popover";

type AgentOutdatedTooltipProps = {
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
    <HelpTooltip>
      <PopoverTrigger>
        <span role="status" css={{ cursor: "pointer" }}>
          {status === agentVersionStatus.Outdated ? "Outdated" : "Deprecated"}
        </span>
      </PopoverTrigger>
      <HelpTooltipContent>
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
      </HelpTooltipContent>
    </HelpTooltip>
  );
};
