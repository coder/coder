import type { AgentTimeReport, AgentTimeRequest } from "#/api/typesGenerated";
import type { UsePaginatedQueryOptions } from "#/hooks/usePaginatedQuery";
import { API } from "../api";

type AgentTimeQueryOptions = Readonly<{
	organization?: string;
	searchParams?: URLSearchParams;
}>;

type AgentTimeQueryPayload = Readonly<{
	organization?: string;
	request: AgentTimeRequest;
}>;

type AgentTimeQueryKey = readonly ["agentTime", AgentTimeQueryPayload, number];

const agentTimeQueryKey = (
	payload: AgentTimeQueryPayload,
	pageNumber: number,
): AgentTimeQueryKey => ["agentTime", payload, pageNumber];

function withoutOrganizationFilter(
	request: AgentTimeRequest,
): AgentTimeRequest {
	const { organization_id: _organizationId, ...organizationRequest } = request;
	return organizationRequest;
}

export const agentTime = (
	request: AgentTimeRequest,
	options: AgentTimeQueryOptions = {},
): UsePaginatedQueryOptions<
	AgentTimeReport,
	AgentTimeQueryPayload,
	unknown,
	AgentTimeReport,
	AgentTimeQueryKey
> => {
	return {
		searchParams: options.searchParams,
		queryPayload: ({ limit, offset }) => {
			const payload = {
				request: {
					...request,
					limit,
					offset,
				},
			};
			return options.organization === undefined
				? payload
				: { ...payload, organization: options.organization };
		},
		queryKey: ({ payload, pageNumber }) =>
			agentTimeQueryKey(payload, pageNumber),
		queryFn: ({ payload, signal }) => {
			if (payload.organization !== undefined) {
				return API.getOrganizationAgentTime(
					payload.organization,
					withoutOrganizationFilter(payload.request),
					signal,
				);
			}

			return API.getAgentTime(payload.request, signal);
		},
		staleTime: 60_000,
	};
};
