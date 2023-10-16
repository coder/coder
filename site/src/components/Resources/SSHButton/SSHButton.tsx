import Popover from "@mui/material/Popover";
import { css } from "@emotion/css";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import { type FC, type PropsWithChildren, useRef, useState } from "react";
import {
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
} from "components/HelpTooltip/HelpTooltip";
import { docs } from "utils/docs";
import { CodeExample } from "../../CodeExample/CodeExample";
import { Stack } from "../../Stack/Stack";
import { SecondaryAgentButton } from "../AgentButton";

export interface SSHButtonProps {
  workspaceName: string;
  agentName: string;
  defaultIsOpen?: boolean;
  sshPrefix?: string;
}

export const SSHButton: FC<PropsWithChildren<SSHButtonProps>> = ({
  workspaceName,
  agentName,
  defaultIsOpen = false,
  sshPrefix,
}) => {
  const theme = useTheme();
  const anchorRef = useRef<HTMLButtonElement>(null);
  const [isOpen, setIsOpen] = useState(defaultIsOpen);
  const id = isOpen ? "schedule-popover" : undefined;

  const onClose = () => {
    setIsOpen(false);
  };

  return (
    <>
      <SecondaryAgentButton
        ref={anchorRef}
        onClick={() => {
          setIsOpen(true);
        }}
      >
        SSH
      </SecondaryAgentButton>

      <Popover
        classes={{
          paper: css`
            padding: ${theme.spacing(2, 3, 3)};
            width: ${theme.spacing(38)};
            color: ${theme.palette.text.secondary};
            margin-top: ${theme.spacing(0.25)};
          `,
        }}
        id={id}
        open={isOpen}
        anchorEl={anchorRef.current}
        onClose={onClose}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "left",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "left",
        }}
      >
        <HelpTooltipText>
          Run the following commands to connect with SSH:
        </HelpTooltipText>

        <Stack spacing={0.5} css={styles.codeExamples}>
          <div>
            <HelpTooltipText>
              <strong css={styles.codeExampleLabel}>
                Configure SSH hosts on machine:
              </strong>
            </HelpTooltipText>
            <CodeExample code="coder config-ssh" />
          </div>

          <div>
            <HelpTooltipText>
              <strong css={styles.codeExampleLabel}>
                Connect to the agent:
              </strong>
            </HelpTooltipText>
            <CodeExample
              code={`ssh ${sshPrefix}${workspaceName}.${agentName}`}
            />
          </div>
        </Stack>

        <HelpTooltipLinksGroup>
          <HelpTooltipLink href={docs("/install")}>
            Install Coder CLI
          </HelpTooltipLink>
          <HelpTooltipLink href={docs("/ides#vs-code-remote")}>
            Connect via VS Code Remote SSH
          </HelpTooltipLink>
          <HelpTooltipLink href={docs("/ides#jetbrains-gateway")}>
            Connect via JetBrains Gateway
          </HelpTooltipLink>
          <HelpTooltipLink href={docs("/ides#ssh-configuration")}>
            SSH configuration
          </HelpTooltipLink>
        </HelpTooltipLinksGroup>
      </Popover>
    </>
  );
};

const styles = {
  codeExamples: (theme) => ({
    marginTop: theme.spacing(1.5),
  }),

  codeExampleLabel: {
    fontSize: 12,
  },
} satisfies Record<string, Interpolation<Theme>>;
