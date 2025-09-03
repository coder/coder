import { deploymentSSHConfig } from "api/queries/deployment";
import { Button } from "components/Button/Button";
import { CodeExample } from "components/CodeExample/CodeExample";
import {
	HelpTooltipLink,
	HelpTooltipLinksGroup,
	HelpTooltipText,
} from "components/HelpTooltip/HelpTooltip";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { Stack } from "components/Stack/Stack";
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
	const { data } = useQuery(deploymentSSHConfig());
	const sshSuffix = data?.hostname_suffix;

	return (
		<Popover>
			<PopoverTrigger asChild={true}>
				<Button size="sm" variant="subtle">
					Connect via SSH
					<ChevronDownIcon />
				</Button>
			</PopoverTrigger>

			<PopoverContent
				align="end"
				className="py-4 px-6 w-80 text-content-secondary mt-[2px] bg-surface-secondary"
			>
				<HelpTooltipText>
					Run the following commands to connect with SSH:
				</HelpTooltipText>

				<ol style={{ margin: 0, padding: 0 }}>
					<Stack spacing={0.5} className="mt-3">
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
			<strong className="text-xs">{helpText}</strong>
		</HelpTooltipText>
		<CodeExample secret={false} code={codeExample} />
	</li>
);
