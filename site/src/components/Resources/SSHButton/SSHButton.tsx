import { css } from "@emotion/css";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import { type FC, type PropsWithChildren } from "react";
import {
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
} from "components/HelpTooltip/HelpTooltip";
import { docs } from "utils/docs";
import { CodeExample } from "../../CodeExample/CodeExample";
import { Stack } from "../../Stack/Stack";
import { SecondaryAgentButton } from "../AgentButton";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "components/Popover/Popover";

export interface SSHButtonProps {
  workspaceName: string;
  agentName: string;
  isDefaultOpen?: boolean;
  sshPrefix?: string;
}

export const SSHButton: FC<PropsWithChildren<SSHButtonProps>> = ({
  workspaceName,
  agentName,
  isDefaultOpen = false,
  sshPrefix,
}) => {
  const theme = useTheme();

  return (
    <Popover isDefaultOpen={isDefaultOpen}>
      <PopoverTrigger>
        <SecondaryAgentButton>SSH</SecondaryAgentButton>
      </PopoverTrigger>

      <PopoverContent
        horizontal="right"
        classes={{
          paper: css`
            padding: ${theme.spacing(2, 3, 3)};
            width: ${theme.spacing(38)};
            color: ${theme.palette.text.secondary};
            margin-top: ${theme.spacing(0.25)};
          `,
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
      </PopoverContent>
    </Popover>
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
