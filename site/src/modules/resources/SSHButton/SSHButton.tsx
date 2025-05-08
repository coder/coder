import type { Interpolation, Theme } from "@emotion/react";
import { ChevronDownIcon } from "lucide-react";
import Button from "@mui/material/Button";
import { CodeExample } from "components/CodeExample/CodeExample";
import {
	HelpTooltipLink,
	HelpTooltipLinksGroup,
	HelpTooltipText,
} from "components/HelpTooltip/HelpTooltip";
import { Stack } from "components/Stack/Stack";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/deprecated/Popover/Popover";
import { type ClassName, useClassName } from "hooks/useClassName";
import type { FC } from "react";
import { docs } from "utils/docs";

export interface AgentSSHButtonProps {
	workspaceName: string;
	agentName: string;
	sshPrefix?: string;
}

export const AgentSSHButton: FC<AgentSSHButtonProps> = ({
	workspaceName,
	agentName,
	sshPrefix,
}) => {
	const paper = useClassName(classNames.paper, []);

	return (
		<Popover>
			<PopoverTrigger>
				<Button
					size="small"
					variant="text"
					endIcon={<ChevronDownIcon className="size-4" />}
					css={{ fontSize: 13, padding: "8px 12px" }}
				>
					Connect via SSH
				</Button>
			</PopoverTrigger>

			<PopoverContent horizontal="right" classes={{ paper }}>
				<HelpTooltipText>
					Run the following commands to connect with SSH:
				</HelpTooltipText>

				<ol style={{ margin: 0, padding: 0 }}>
					<Stack spacing={0.5} css={styles.codeExamples}>
						<SSHStep
							helpText="Configure SSH hosts on machine:"
							codeExample="coder config-ssh"
						/>
						<SSHStep
							helpText="Connect to the agent:"
							codeExample={`ssh ${sshPrefix}${workspaceName}.${agentName}`}
						/>
					</Stack>
				</ol>

				<HelpTooltipLinksGroup>
					<HelpTooltipLink href={docs("/install")}>
						Install Coder CLI
					</HelpTooltipLink>
					<HelpTooltipLink href={docs("/user-guides/workspace-access/vscode")}>
						Connect via VS Code Remote SSH
					</HelpTooltipLink>
					<HelpTooltipLink
						href={docs("/user-guides/workspace-access/jetbrains")}
					>
						Connect via JetBrains Gateway
					</HelpTooltipLink>
					<HelpTooltipLink href={docs("/user-guides/workspace-access#ssh")}>
						SSH configuration
					</HelpTooltipLink>
				</HelpTooltipLinksGroup>
			</PopoverContent>
		</Popover>
	);
};

export interface AgentDevcontainerSSHButtonProps {
	workspace: string;
	container: string;
}

export const AgentDevcontainerSSHButton: FC<
	AgentDevcontainerSSHButtonProps
> = ({ workspace, container }) => {
	const paper = useClassName(classNames.paper, []);

	return (
		<Popover>
			<PopoverTrigger>
				<Button
					size="small"
					variant="text"
					endIcon={<ChevronDownIcon className="size-4" />}
					css={{ fontSize: 13, padding: "8px 12px" }}
				>
					Connect via SSH
				</Button>
			</PopoverTrigger>

			<PopoverContent horizontal="right" classes={{ paper }}>
				<HelpTooltipText>
					Run the following commands to connect with SSH:
				</HelpTooltipText>

				<ol style={{ margin: 0, padding: 0 }}>
					<Stack spacing={0.5} css={styles.codeExamples}>
						<SSHStep
							helpText="Connect to the container:"
							codeExample={`coder ssh ${workspace} -c ${container}`}
						/>
					</Stack>
				</ol>

				<HelpTooltipLinksGroup>
					<HelpTooltipLink href={docs("/install")}>
						Install Coder CLI
					</HelpTooltipLink>
					<HelpTooltipLink href={docs("/user-guides/workspace-access#ssh")}>
						SSH configuration
					</HelpTooltipLink>
				</HelpTooltipLinksGroup>
			</PopoverContent>
		</Popover>
	);
};

interface SSHStepProps {
	helpText: string;
	codeExample: string;
}

const SSHStep: FC<SSHStepProps> = ({ helpText, codeExample }) => (
	<li style={{ listStylePosition: "inside" }}>
		<HelpTooltipText style={{ display: "inline" }}>
			<strong css={styles.codeExampleLabel}>{helpText}</strong>
		</HelpTooltipText>
		<CodeExample secret={false} code={codeExample} />
	</li>
);

const classNames = {
	paper: (css, theme) => css`
    padding: 16px 24px 24px;
    width: 304px;
    color: ${theme.palette.text.secondary};
    margin-top: 2px;
  `,
} satisfies Record<string, ClassName>;

const styles = {
	codeExamples: {
		marginTop: 12,
	},

	codeExampleLabel: {
		fontSize: 12,
	},
} satisfies Record<string, Interpolation<Theme>>;
