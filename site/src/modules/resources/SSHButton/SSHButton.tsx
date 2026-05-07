import type { FC } from "react";
import { useQuery } from "react-query";
import { deploymentSSHConfig } from "#/api/queries/deployment";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import { CodeExample } from "#/components/CodeExample/CodeExample";
import {
	HelpPopoverLink,
	HelpPopoverLinksGroup,
	HelpPopoverText,
} from "#/components/HelpPopover/HelpPopover";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { docs } from "#/utils/docs";

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
				<HelpPopoverText>
					Run the following commands to connect with SSH:
				</HelpPopoverText>

				<ol style={{ margin: 0, padding: 0 }}>
					<div className="flex flex-col gap-1 mt-3">
						<SSHStep
							helpText="Configure SSH hosts on machine:"
							codeExample="coder config-ssh"
						/>
						<SSHStep
							helpText="Connect to the agent:"
							codeExample={`ssh ${agentName}.${workspaceName}.${workspaceOwnerUsername}.${sshSuffix}`}
						/>
					</div>
				</ol>

				<HelpPopoverLinksGroup>
					<HelpPopoverLink href={docs("/install")}>
						Install Coder CLI
					</HelpPopoverLink>
					<HelpPopoverLink href={docs("/user-guides/workspace-access/vscode")}>
						Connect via VS Code Remote SSH
					</HelpPopoverLink>
					<HelpPopoverLink
						href={docs("/user-guides/workspace-access/jetbrains")}
					>
						Connect via JetBrains IDEs
					</HelpPopoverLink>
					<HelpPopoverLink href={docs("/user-guides/desktop")}>
						Connect via Coder Desktop
					</HelpPopoverLink>
					<HelpPopoverLink href={docs("/user-guides/workspace-access#ssh")}>
						SSH configuration
					</HelpPopoverLink>
				</HelpPopoverLinksGroup>
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
		<HelpPopoverText style={{ display: "inline" }}>
			<strong className="text-xs">{helpText}</strong>
		</HelpPopoverText>
		<CodeExample secret={false} code={codeExample} />
	</li>
);
