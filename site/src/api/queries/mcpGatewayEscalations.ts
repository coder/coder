import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import { getErrorStatus } from "#/api/errors";
import type { MCPGatewayEscalationStatus } from "#/api/typesGenerated";

const mcpGatewayEscalationsKey = ["mcpGateway", "escalations"] as const;

export const mcpGatewayEscalations = (status?: MCPGatewayEscalationStatus) => ({
	queryKey: [...mcpGatewayEscalationsKey, status ?? "all"] as const,
	queryFn: () => API.getMCPGatewayEscalations(status),
});

const invalidateMCPGatewayEscalations = (queryClient: QueryClient) =>
	queryClient.invalidateQueries({ queryKey: mcpGatewayEscalationsKey });

export const approveMCPGatewayEscalation = (queryClient: QueryClient) => ({
	mutationFn: (id: string) => API.approveMCPGatewayEscalation(id),
	onSuccess: () => invalidateMCPGatewayEscalations(queryClient),
	onError: (error: unknown) => {
		if (getErrorStatus(error) === 409) {
			return invalidateMCPGatewayEscalations(queryClient);
		}
	},
});

export const denyMCPGatewayEscalation = (queryClient: QueryClient) => ({
	mutationFn: (id: string) => API.denyMCPGatewayEscalation(id),
	onSuccess: () => invalidateMCPGatewayEscalations(queryClient),
	onError: (error: unknown) => {
		if (getErrorStatus(error) === 409) {
			return invalidateMCPGatewayEscalations(queryClient);
		}
	},
});
