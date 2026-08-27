import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	approveMCPGatewayEscalation,
	denyMCPGatewayEscalation,
	mcpGatewayEscalations,
} from "#/api/queries/mcpGatewayEscalations";
import { pageTitle } from "#/utils/page";
import { MCPEscalationsPageView } from "./MCPEscalationsPageView";

interface MCPEscalationsPageProps {
	referenceDate?: Date;
}

const MCPEscalationsPage: FC<MCPEscalationsPageProps> = ({
	referenceDate = new Date(),
}) => {
	const queryClient = useQueryClient();
	const escalationsQuery = useQuery({
		...mcpGatewayEscalations("pending"),
		refetchInterval: 5_000,
	});
	const approveMutation = useMutation(approveMCPGatewayEscalation(queryClient));
	const denyMutation = useMutation(denyMCPGatewayEscalation(queryClient));
	const escalations = (escalationsQuery.data ?? []).toSorted(
		(a, b) =>
			new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
	);

	return (
		<>
			<title>{pageTitle("Tool call approvals")}</title>
			<MCPEscalationsPageView
				escalations={escalations}
				isLoading={escalationsQuery.isLoading}
				queryError={escalationsQuery.error}
				mutationError={approveMutation.error ?? denyMutation.error}
				approvingId={
					approveMutation.isPending ? approveMutation.variables : undefined
				}
				denyingId={denyMutation.isPending ? denyMutation.variables : undefined}
				referenceDate={referenceDate}
				onApprove={(id) => {
					approveMutation.reset();
					denyMutation.reset();
					approveMutation.mutate(id);
				}}
				onDeny={(id) => {
					approveMutation.reset();
					denyMutation.reset();
					denyMutation.mutate(id);
				}}
			/>
		</>
	);
};

export default MCPEscalationsPage;
