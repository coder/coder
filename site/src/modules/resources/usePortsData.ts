import { useQuery } from "react-query";
import { workspacePortShares } from "#/api/queries/workspaceportsharing";
import { agentListeningPorts } from "#/api/queries/workspaces";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceAgentListeningPort,
	WorkspaceAgentPortShare,
} from "#/api/typesGenerated";
import { getWorkspaceListeningPortsProtocol } from "#/utils/portForward";

/**
 * Whether port-forwarding UI (ports menus, port preview tabs) should be shown
 * for the agent: requires a configured wildcard access URL and the agent
 * opting into the port_forwarding_helper display app.
 */
export const canShowPortForwarding = (
	agent: WorkspaceAgent,
	host: string,
): boolean => {
	return (
		host.trim() !== "" && agent.display_apps.includes("port_forwarding_helper")
	);
};

export interface PortsData {
	listeningPorts: readonly WorkspaceAgentListeningPort[] | undefined;
	sharedPorts: readonly WorkspaceAgentPortShare[] | undefined;
	privateListeningPorts: readonly WorkspaceAgentListeningPort[];
	totalCount: number | undefined;
	protocol: "http" | "https";
	refetchSharedPorts: () => void;
}

/**
 * Used by both the workspace port-forward button and the agents page right-panel
 * ports menu so they stay on the same query keys and refresh cadence.
 */
export const usePortsData = (
	workspace: Workspace,
	agent: WorkspaceAgent,
	enabled: boolean,
): PortsData => {
	const protocol = getWorkspaceListeningPortsProtocol(workspace.id);

	const { data: listeningPorts } = useQuery({
		...agentListeningPorts(agent.id),
		enabled,
		refetchInterval: enabled ? 5_000 : false,
		staleTime: 0,
		select: (res) => res.ports,
	});

	const { data: sharedPorts, refetch: refetchSharedPorts } = useQuery({
		...workspacePortShares(workspace.id),
		enabled,
		staleTime: 0,
		select: (res) => res.shares.filter((s) => s.agent_name === agent.name),
	});

	// Listening ports that haven't been explicitly shared appear in their own
	// section; shared ports bubble up to the "Shared" section.
	const sharedPortNumbers = new Set((sharedPorts ?? []).map((s) => s.port));
	const privateListeningPorts = (listeningPorts ?? []).filter(
		(p) => !sharedPortNumbers.has(p.port),
	);

	const totalCount =
		listeningPorts !== undefined ? listeningPorts.length : undefined;

	return {
		listeningPorts,
		sharedPorts,
		privateListeningPorts,
		totalCount,
		protocol,
		refetchSharedPorts,
	};
};
