import { useQuery } from "react-query";
import { deploymentSSHConfig } from "#/api/queries/deployment";
import type { Workspace } from "#/api/typesGenerated";
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
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { docs } from "#/utils/docs";
import { getWorkspaceAgents } from "#/utils/workspace";

type AgentSSHButtonProps = {
	workspace: Workspace;
	agentName: string;
};

export const AgentSSHButton: React.FC<AgentSSHButtonProps> = ({
	workspace,
	agentName,
}) => {
	const { user } = useAuthenticated();
	const { data, isLoading, error } = useQuery(deploymentSSHConfig());
	const sshSuffix = data?.hostname_suffix;
	const configCommand = sshSuffix ? "coder config-ssh" : undefined;
	const isSingleAgent = getWorkspaceAgents(workspace).length === 1;
	let connectCommand: string | undefined;
	if (!isLoading) {
		if (sshSuffix) {
			const shortHostname = `${workspace.name}.${sshSuffix}`;
			const hostnamePrefix = data?.hostname_prefix;
			// SSH selects legacy prefix blocks before suffix blocks.
			const matchesLegacyPrefix = hostnamePrefix
				? shortHostname.toLowerCase().startsWith(hostnamePrefix.toLowerCase())
				: false;
			// Shared workspaces need the full hostname; SSH has no workspace.owner form.
			connectCommand =
				isSingleAgent && user.id === workspace.owner_id && !matchesLegacyPrefix
					? `ssh ${shortHostname}`
					: `ssh ${agentName}.${workspace.name}.${workspace.owner_name}.${sshSuffix}`;
		} else {
			// The CLI needs no SSH config, so it remains usable without a suffix.
			connectCommand = isSingleAgent
				? `coder ssh ${workspace.owner_name}/${workspace.name}`
				: `coder ssh ${workspace.owner_name}/${workspace.name}.${agentName}`;
		}
	}

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
				className="py-4 px-6 w-80 text-content-secondary mt-0.5 bg-surface-secondary"
			>
				<HelpPopoverText>
					Run the following commands to connect with SSH:
				</HelpPopoverText>

				{error && (
					<ErrorAlert error={error} showDebugDetail={false} className="mt-3" />
				)}

				<Spinner loading={connectCommand === undefined} className="mt-3">
					<ol style={{ margin: 0, padding: 0 }}>
						<div className="flex flex-col gap-1 mt-3">
							{configCommand && (
								<SSHStep
									helpText="Configure SSH hosts on machine:"
									codeExample={configCommand}
								/>
							)}
							{connectCommand && (
								<SSHStep
									helpText="Connect to the agent:"
									codeExample={connectCommand}
								/>
							)}
						</div>
					</ol>
				</Spinner>

				<HelpPopoverLinksGroup>
					<HelpPopoverLink href="/install">Install Coder CLI</HelpPopoverLink>
					<HelpPopoverLink href={docs("/workspaces/workspace-access/vscode")}>
						Connect via VS Code Remote SSH
					</HelpPopoverLink>
					<HelpPopoverLink
						href={docs("/workspaces/workspace-access/jetbrains")}
					>
						Connect via JetBrains IDEs
					</HelpPopoverLink>
					<HelpPopoverLink href={docs("/workspaces/desktop")}>
						Connect via Coder Desktop
					</HelpPopoverLink>
					<HelpPopoverLink href={docs("/workspaces/workspace-access#ssh")}>
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
