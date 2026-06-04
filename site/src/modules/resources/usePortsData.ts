import { useQuery } from "react-query";
import { API } from "#/api/api";
import { workspacePortShares } from "#/api/queries/workspaceportsharing";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceAgentListeningPort,
	WorkspaceAgentPortShare,
} from "#/api/typesGenerated";
import { getWorkspaceListeningPortsProtocol } from "#/utils/portForward";

export interface PortsData {
	listeningPorts: readonly WorkspaceAgentListeningPort[] | undefined;
	sharedPorts: readonly WorkspaceAgentPortShare[] | undefined;
	privateListeningPorts: readonly WorkspaceAgentListeningPort[];
	totalCount: number | undefined;
	protocol: "http" | "https";
	refetchSharedPorts: () => void;
}

/**
 * Fetches an agent's listening ports and the workspace's shared ports, filtered
 * to the given agent. Shared by the workspace port-forward button and the
 * AgentsPage right-panel ports menu so both stay on the same query keys and
 * refresh cadence.
 */
export const usePortsData = (
	workspace: Workspace,
	agent: WorkspaceAgent,
	enabled: boolean,
): PortsData => {
	const protocol = getWorkspaceListeningPortsProtocol(workspace.id);

	const { data: listeningPorts } = useQuery({
		queryKey: ["portForward", agent.id],
		queryFn: () => API.getAgentListeningPorts(agent.id),
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
