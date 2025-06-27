import type { Interpolation, Theme } from "@emotion/react";
import { deploymentSSHConfig } from "api/queries/deployment";
import { Button } from "components/Button/Button";
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
import { ChevronDownIcon } from "lucide-react";
import type { FC } from "react";
import { useQuery } from "react-query";
import { docs } from "utils/docs";

interface AgentSSHButtonProps {
	workspaceName: string;
	agentName: string;
	workspaceOwnerUsername: string;
}

export const AgentSSHButton: FC<AgentSSHButtonProps> = ({
	workspaceName,
	agentName,
	workspaceOwnerUsername,
}) => {
	const paper = useClassName(classNames.paper, []);
	const { data } = useQuery(deploymentSSHConfig());
	const sshSuffix = data?.hostname_suffix;

	return (
		<Popover>
			<PopoverTrigger>
				<Button size="sm" variant="subtle">
					Connect via SSH
					<ChevronDownIcon />
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
							codeExample={`ssh ${agentName}.${workspaceName}.${workspaceOwnerUsername}.${sshSuffix}`}
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
						Connect via JetBrains IDEs
					</HelpTooltipLink>
					<HelpTooltipLink href={docs("/user-guides/desktop")}>
						Connect via Coder Desktop
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
