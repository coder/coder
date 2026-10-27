import { useQuery } from "react-query";
import { deploymentSSHConfig } from "#/api/queries/deployment";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
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
import { Spinner } from "#/components/Spinner/Spinner";
import { docs } from "#/utils/docs";

type AgentSSHButtonProps = {
	workspaceName: string;
	agentName: string;
	workspaceOwnerUsername: string;
};

export const AgentSSHButton: React.FC<AgentSSHButtonProps> = ({
	workspaceName,
	agentName,
	workspaceOwnerUsername,
}) => {
	const { data, isLoading, error } = useQuery(deploymentSSHConfig());
	const sshSuffix = data?.hostname_suffix;
	// The CLI needs no SSH config, so it remains usable when the suffix is
	// empty or the deployment SSH config could not be loaded.
	const connectCommand = sshSuffix
		? `ssh ${agentName}.${workspaceName}.${workspaceOwnerUsername}.${sshSuffix}`
		: `coder ssh ${workspaceOwnerUsername}/${workspaceName}.${agentName}`;

	return (
		<Popover>
			<PopoverTrigger asChild>
				<Button size="sm" variant="subtle">
					Connect via SSH
					<ChevronDownIcon />
				</Button>
			</PopoverTrigger>

			<PopoverContent
				onOpenAutoFocus={(event) => event.preventDefault()}
				align="end"
				className="py-4 px-6 w-80 text-content-secondary mt-[2px] bg-surface-secondary"
			>
				<HelpPopoverText>
					Run the following commands to connect with SSH:
				</HelpPopoverText>

				{error && (
					<ErrorAlert error={error} showDebugDetail={false} className="mt-3" />
				)}

				<Spinner loading={isLoading} className="mt-3">
					<ol style={{ margin: 0, padding: 0 }}>
						<div className="flex flex-col gap-1 mt-3">
							{sshSuffix && (
								<SSHStep
									helpText="Configure SSH hosts on machine:"
									codeExample="coder config-ssh"
								/>
							)}
							<SSHStep
								helpText="Connect to the agent:"
								codeExample={connectCommand}
							/>
						</div>
					</ol>
				</Spinner>

				<HelpPopoverLinksGroup>
					<HelpPopoverLink href="/install">Install Coder CLI</HelpPopoverLink>
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

type SSHStepProps = {
	helpText: string;
	codeExample: string;
};

const SSHStep: React.FC<SSHStepProps> = ({ helpText, codeExample }) => (
	<li style={{ listStylePosition: "inside" }}>
		<HelpPopoverText style={{ display: "inline" }}>
			<strong className="text-xs">{helpText}</strong>
		</HelpPopoverText>
		<CodeExample secret={false} code={codeExample} />
	</li>
);
