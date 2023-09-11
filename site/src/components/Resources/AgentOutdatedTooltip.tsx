import { ComponentProps, FC } from "react";
import { makeStyles } from "@mui/styles";
import RefreshIcon from "@mui/icons-material/RefreshOutlined";
import {
  HelpTooltipText,
  HelpPopover,
  HelpTooltipTitle,
  HelpTooltipAction,
  HelpTooltipLinksGroup,
  HelpTooltipContext,
} from "components/HelpTooltip/HelpTooltip";
import { WorkspaceAgent } from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";

type AgentOutdatedTooltipProps = ComponentProps<typeof HelpPopover> & {
  agent: WorkspaceAgent;
  serverVersion: string;
  onUpdate: () => void;
};

export const AgentOutdatedTooltip: FC<AgentOutdatedTooltipProps> = ({
  agent,
  serverVersion,
  onUpdate,
  onOpen,
  id,
  open,
  onClose,
  anchorEl,
}) => {
  const styles = useStyles();

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
            <HelpTooltipTitle>Agent Outdated</HelpTooltipTitle>
            <HelpTooltipText>
              This agent is an older version than the Coder server. This can
              happen after you update Coder with running workspaces. To fix
              this, you can stop and start the workspace.
            </HelpTooltipText>
          </div>

          <Stack spacing={0.5}>
            <span className={styles.versionLabel}>Agent version</span>
            <span>{agent.version}</span>
          </Stack>

          <Stack spacing={0.5}>
            <span className={styles.versionLabel}>Server version</span>
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

const useStyles = makeStyles((theme) => ({
  versionLabel: {
    fontWeight: 600,
    color: theme.palette.text.primary,
  },
}));
