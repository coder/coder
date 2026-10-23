import { mutationOptions, type QueryClient } from "react-query";
import { API } from "#/api/api";
import type {
	CreateTemplateResourceRevisionRequest,
	CreateTemplateResourceRunRequest,
} from "#/api/typesGenerated";
import { templateKey } from "./templates";

/** Template-scoped Terraform state, independent of workspace preview resources. */
export const templateResourcesKey = (templateId: string) =>
	[...templateKey(templateId), "resources"] as const;

/** Polls state and history independently of workspace build status. */
export const templateResources = (templateId: string) => ({
	queryKey: templateResourcesKey(templateId),
	queryFn: () => API.getTemplateResources(templateId),
	refetchInterval: 5000,
});

/** Keeps each run's retained output separate from the current resource state. */
export const templateResourceRunLogsKey = (templateId: string, runId: string) =>
	[...templateResourcesKey(templateId), "runs", runId, "logs"] as const;

/** Polls persisted output without depending on a live provisioner connection. */
export const templateResourceRunLogs = (templateId: string, runId: string) => ({
	queryKey: templateResourceRunLogsKey(templateId, runId),
	queryFn: () => API.getTemplateResourceRunLogs(templateId, runId),
	refetchInterval: 5000,
});

/** Refreshes revision history even when the save response is inconclusive. */
export const createTemplateResourceRevision = (
	queryClient: QueryClient,
	templateId: string,
) =>
	mutationOptions({
		mutationFn: (request: CreateTemplateResourceRevisionRequest) =>
			API.createTemplateResourceRevision(templateId, request),
		onSettled: () =>
			queryClient.invalidateQueries({
				queryKey: templateResourcesKey(templateId),
				exact: true,
			}),
	});

/** Refreshes job history after an attempt, without refetching historical logs. */
export const createTemplateResourceRun = (
	queryClient: QueryClient,
	templateId: string,
) =>
	mutationOptions({
		mutationFn: (request: CreateTemplateResourceRunRequest) =>
			API.createTemplateResourceRun(templateId, request),
		onSettled: () =>
			queryClient.invalidateQueries({
				queryKey: templateResourcesKey(templateId),
				exact: true,
			}),
	});
