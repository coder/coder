import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

const mcpServerProposalKey = (proposalId: string) =>
	["mcp-server-proposals", proposalId] as const;

export const mcpServerProposal = (proposalId: string) => ({
	queryKey: mcpServerProposalKey(proposalId),
	queryFn: (): Promise<TypesGen.MCPServerProposal> =>
		API.getMCPServerProposal(proposalId),
});

export const acceptMCPServerProposal = (
	queryClient: QueryClient,
	proposalId: string,
) => ({
	mutationFn: (): Promise<TypesGen.AcceptMCPServerProposalResponse> =>
		API.acceptMCPServerProposal(proposalId),
	onSuccess: async () => {
		await queryClient.invalidateQueries({
			queryKey: mcpServerProposalKey(proposalId),
		});
	},
});

export const rejectMCPServerProposal = (proposalId: string) => ({
	mutationFn: (): Promise<void> => API.rejectMCPServerProposal(proposalId),
});
