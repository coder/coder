import { mutationOptions, type QueryClient } from "react-query";
import { API } from "#/api/api";
import type {
	AuthorizationRequest,
	CreateFoundationRequest,
	CreateTemplateResourceRevisionRequest,
	CreateTemplateResourceRunRequest,
	SetTemplateFoundationRequest,
	UpdateFoundationRequest,
} from "#/api/typesGenerated";
import { checkAuthorization, getAuthorizationKey } from "./authCheck";
import { provisionerJobsQueryKey } from "./organizations";

const templateFoundationsKey = ["template-foundations"] as const;

/** Namespace for organization-owned deployments, including unbound Foundations. */
export const foundationsKey = (organization: string) =>
	["organizations", organization, "foundations"] as const;
/** Stable identity shared by standalone management and execution history. */
export const foundationKey = (organization: string, id: string) =>
	[...foundationsKey(organization), id] as const;
/** Association cache, separate from workspace template versions. */
export const templateFoundationKey = (templateId: string) =>
	[...templateFoundationsKey, templateId] as const;
/** Lists accessible Foundations, not just those attached to templates. */
export const foundations = (organization: string) => ({
	queryKey: foundationsKey(organization),
	queryFn: () => API.getFoundations(organization),
});
/** Polls persisted execution state independently of workspace builds. */
export const foundation = (organization: string, id: string) => ({
	queryKey: foundationKey(organization, id),
	queryFn: () => API.getFoundation(organization, id),
	refetchInterval: 5000,
});
/** A null response means the template has no Foundation association. */
export const templateFoundation = (templateId: string) => ({
	queryKey: templateFoundationKey(templateId),
	queryFn: () => API.getTemplateFoundation(templateId),
	refetchInterval: 5000,
});
/** Isolates retained log output by Foundation and immutable run identity. */
export const foundationRunLogsKey = (
	organization: string,
	id: string,
	runId: string,
) => [...foundationKey(organization, id), "runs", runId, "logs"] as const;
/** Reads retained logs without depending on a live provisioner connection. */
export const foundationRunLogs = (
	organization: string,
	id: string,
	runId: string,
) => ({
	queryKey: foundationRunLogsKey(organization, id, runId),
	queryFn: () => API.getFoundationRunLogs(organization, id, runId),
	refetchInterval: 5000,
});

const refreshFoundations = async (
	client: QueryClient,
	organization: string,
	id?: string,
) => {
	await client.invalidateQueries({
		queryKey: foundationsKey(organization),
		exact: true,
	});
	if (!id) return;
	await Promise.all([
		client.invalidateQueries({
			queryKey: foundationKey(organization, id),
			exact: true,
		}),
		// Association responses embed the same aggregate, not just its ID.
		client.invalidateQueries({
			queryKey: templateFoundationsKey,
			predicate: ({ state }) => {
				const data = state.data;
				return (
					data !== null &&
					typeof data === "object" &&
					"id" in data &&
					data.id === id
				);
			},
		}),
	]);
};

/** Creation saves settings only and never enqueues a Terraform operation. */
export const createFoundation = (client: QueryClient, organization: string) =>
	mutationOptions({
		mutationFn: (request: CreateFoundationRequest) =>
			API.createFoundation(organization, request),
		onSettled: () => refreshFoundations(client, organization),
	});
/** Refreshes settings without replacing local editor drafts. */
export const updateFoundation = (
	client: QueryClient,
	organization: string,
	id: string,
) =>
	mutationOptions({
		mutationFn: (request: UpdateFoundationRequest) =>
			API.updateFoundation(organization, id, request),
		onSettled: () => refreshFoundations(client, organization, id),
	});
/** Binding is separate from creation, so failure must retain the created entity. */
export const setTemplateFoundation = (
	client: QueryClient,
	organization: string,
	templateId: string,
) =>
	mutationOptions({
		mutationFn: async (request: SetTemplateFoundationRequest) => {
			const permissions = await client.fetchQuery(
				foundationPermissions(organization, request.foundation_id),
			);
			if (!permissions.canUseFoundation)
				throw new Error(
					"Foundation use permission is required to attach this Foundation.",
				);
			return API.setTemplateFoundation(templateId, request);
		},
		onSettled: (_data, _error, request) =>
			Promise.all([
				refreshFoundations(client, organization, request.foundation_id),
				client.invalidateQueries({
					queryKey: templateFoundationKey(templateId),
					exact: true,
				}),
			]),
	});
/** Refreshes revision history even if saving returns an inconclusive error. */
export const createFoundationRevision = (
	client: QueryClient,
	organization: string,
	id: string,
) =>
	mutationOptions({
		mutationFn: (request: CreateTemplateResourceRevisionRequest) =>
			API.createFoundationRevision(organization, id, request),
		onSettled: () => refreshFoundations(client, organization, id),
	});
/** Enqueues only the requested Foundation operation, never a workspace build. */
export const createFoundationRun = (
	client: QueryClient,
	organization: string,
	id: string,
) =>
	mutationOptions({
		mutationFn: (request: CreateTemplateResourceRunRequest) =>
			API.createFoundationRun(organization, id, request),
		onSettled: () =>
			Promise.all([
				refreshFoundations(client, organization, id),
				client.invalidateQueries({
					queryKey: provisionerJobsQueryKey(organization),
				}),
			]),
	});

/** Deletes a cleaned-up Foundation without running Terraform. */
export const deleteFoundation = (
	client: QueryClient,
	organization: string,
	id: string,
) =>
	mutationOptions({
		mutationFn: () => API.deleteFoundation(organization, id),
		onSettled: () => refreshFoundations(client, organization, id),
	});

const foundationChecks = (
	organization: string,
	id?: string,
): AuthorizationRequest["checks"] => ({
	canReadFoundation: {
		object: {
			resource_type: "foundation",
			organization_id: organization,
			resource_id: id,
		},
		action: "read",
	},
	canCreateFoundation: {
		object: { resource_type: "foundation", organization_id: organization },
		action: "create",
	},
	canUpdateFoundation: {
		object: {
			resource_type: "foundation",
			organization_id: organization,
			resource_id: id,
		},
		action: "update",
	},
	canDeleteFoundation: {
		object: {
			resource_type: "foundation",
			organization_id: organization,
			resource_id: id,
		},
		action: "delete",
	},
	canUseFoundation: {
		object: {
			resource_type: "foundation",
			organization_id: organization,
			resource_id: id,
		},
		action: "use",
	},
});
/** Organization and object checks must not share permission cache entries. */
export const foundationPermissionsKey = (organization: string, id?: string) =>
	getAuthorizationKey({ checks: foundationChecks(organization, id) });
/** Foundation permissions are independent of template editing permissions. */
export const foundationPermissions = (organization: string, id?: string) =>
	checkAuthorization({ checks: foundationChecks(organization, id) });
/** Management remains reachable without an attached template or workspace version. */
export const foundationPath = (organization: string, id: string) =>
	`/organizations/${encodeURIComponent(organization)}/foundations/${encodeURIComponent(id)}`;
