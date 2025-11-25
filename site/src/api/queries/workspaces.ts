import { API, type DeleteWorkspaceOptions } from "api/api";
import { DetailedError, isApiValidationError } from "api/errors";
import type {
	CreateWorkspaceRequest,
	ProvisionerLogLevel,
	UsageAppName,
	Workspace,
	WorkspaceACL,
	WorkspaceAgentLog,
	WorkspaceBuild,
	WorkspaceBuildParameter,
	WorkspaceRole,
	WorkspacesRequest,
	WorkspacesResponse,
} from "api/typesGenerated";
import type { Dayjs } from "dayjs";
import {
	type WorkspacePermissions,
	workspaceChecks,
} from "modules/workspaces/permissions";
import type { ConnectionStatus } from "pages/TerminalPage/types";
import type {
	MutationOptions,
	QueryClient,
	QueryOptions,
	UseMutationOptions,
	UseQueryOptions,
} from "react-query";
import { checkAuthorization } from "./authCheck";
import { disabledRefetchOptions } from "./util";
import { workspaceBuildsKey } from "./workspaceBuilds";

export const workspaceByOwnerAndNameKey = (
	ownerUsername: string,
	name: string,
) => ["workspace", ownerUsername, name, "settings"];

export const workspaceByOwnerAndName = (owner: string, name: string) => {
	return {
		queryKey: workspaceByOwnerAndNameKey(owner, name),
		queryFn: () =>
			API.getWorkspaceByOwnerAndName(owner, name, {
				include_deleted: true,
			}),
	};
};

export const workspaceACLKey = (workspaceId: string) => [
	"workspaceAcl",
	workspaceId,
];

export const workspaceACL = (workspaceId: string) => {
	return {
		queryKey: workspaceACLKey(workspaceId),
		queryFn: () => API.getWorkspaceACL(workspaceId),
	} satisfies QueryOptions<WorkspaceACL>;
};

export const setWorkspaceUserRole = (
	queryClient: QueryClient,
): MutationOptions<
	void,
	unknown,
	{
		workspaceId: string;
		userId: string;
		role: WorkspaceRole;
	}
> => {
	return {
		mutationFn: ({ workspaceId, userId, role }) =>
			API.updateWorkspaceACL(workspaceId, {
				user_roles: {
					[userId]: role,
				},
			}),
		onSuccess: async (_res, { workspaceId }) => {
			await queryClient.invalidateQueries({
				queryKey: workspaceACLKey(workspaceId),
			});
		},
	};
};

export const setWorkspaceGroupRole = (
	queryClient: QueryClient,
): MutationOptions<
	void,
	unknown,
	{ workspaceId: string; groupId: string; role: WorkspaceRole }
> => {
	return {
		mutationFn: ({ workspaceId, groupId, role }) =>
			API.updateWorkspaceACL(workspaceId, {
				group_roles: {
					[groupId]: role,
				},
			}),
		onSuccess: async (_res, { workspaceId }) => {
			await queryClient.invalidateQueries({
				queryKey: workspaceACLKey(workspaceId),
			});
		},
	};
};

type CreateWorkspaceMutationVariables = CreateWorkspaceRequest & {
	userId: string;
};

export const createWorkspace = (queryClient: QueryClient) => {
	return {
		mutationFn: async (variables: CreateWorkspaceMutationVariables) => {
			const { userId, ...req } = variables;
			return API.createWorkspace(userId, req);
		},
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: ["workspaces"] });
		},
	};
};

type AutoCreateWorkspaceOptions = {
	organizationId: string;
	templateName: string;
	workspaceName: string;
	/**
	 * If provided, the auto-create workspace feature will attempt to find a
	 * matching workspace. If found, it will return the existing workspace instead
	 * of creating a new one. Its value supports [advanced filtering queries for
	 * workspaces](https://coder.com/docs/user-guides/workspace-management#workspace-filtering). If
	 * multiple values are returned, the first one will be returned.
	 */
	match: string | null;
	templateVersionId?: string;
	buildParameters?: WorkspaceBuildParameter[];
};

export const autoCreateWorkspace = (queryClient: QueryClient) => {
	return {
		mutationFn: async ({
			organizationId,
			templateName,
			workspaceName,
			templateVersionId,
			buildParameters,
			match,
		}: AutoCreateWorkspaceOptions) => {
			if (match) {
				const matchWorkspace = await findMatchWorkspace(
					`owner:me template:${templateName} ${match}`,
				);
				if (matchWorkspace) {
					return matchWorkspace;
				}
			}

			let templateVersionParameters: Partial<CreateWorkspaceRequest>;

			if (templateVersionId) {
				templateVersionParameters = { template_version_id: templateVersionId };
			} else {
				const template = await API.getTemplateByName(
					organizationId,
					templateName,
				);
				templateVersionParameters = { template_id: template.id };
			}

			return API.createWorkspace("me", {
				...templateVersionParameters,
				name: workspaceName,
				rich_parameter_values: buildParameters,
			});
		},
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: ["workspaces"] });
		},
	};
};

async function findMatchWorkspace(q: string): Promise<Workspace | undefined> {
	try {
		const { workspaces } = await API.getWorkspaces({ q, limit: 1 });
		const matchWorkspace = workspaces.at(0);
		if (matchWorkspace) {
			return matchWorkspace;
		}
	} catch (err) {
		if (isApiValidationError(err)) {
			const firstValidationErrorDetail =
				err.response.data.validations?.[0].detail;
			throw new DetailedError(
				"Invalid match value",
				firstValidationErrorDetail,
			);
		}

		throw err;
	}
}

function workspacesKey(req: WorkspacesRequest = {}) {
	return ["workspaces", req] as const;
}

export function workspaces(req: WorkspacesRequest = {}) {
	return {
		queryKey: workspacesKey(req),
		queryFn: () => API.getWorkspaces(req),
	} as const satisfies QueryOptions<WorkspacesResponse>;
}

export const updateDeadline = (
	workspace: Workspace,
): UseMutationOptions<void, unknown, Dayjs> => {
	return {
		mutationFn: (deadline: Dayjs) => {
			return API.putWorkspaceExtension(workspace.id, deadline);
		},
	};
};

export const changeVersion = (
	workspace: Workspace,
	queryClient: QueryClient,
	isDynamicParametersEnabled: boolean,
) => {
	return {
		mutationFn: ({
			versionId,
			buildParameters,
		}: {
			versionId: string;
			buildParameters?: WorkspaceBuildParameter[];
		}) => {
			return API.changeWorkspaceVersion(
				workspace,
				versionId,
				buildParameters,
				isDynamicParametersEnabled,
			);
		},
		onSuccess: async (build: WorkspaceBuild) => {
			await updateWorkspaceBuild(build, queryClient);
		},
	};
};

export const updateWorkspace = (
	workspace: Workspace,
	queryClient: QueryClient,
) => {
	return {
		mutationFn: ({
			buildParameters,
			isDynamicParametersEnabled,
		}: {
			buildParameters?: WorkspaceBuildParameter[];
			isDynamicParametersEnabled: boolean;
		}) => {
			return API.updateWorkspace(
				workspace,
				buildParameters,
				isDynamicParametersEnabled,
			);
		},
		onSuccess: async (build: WorkspaceBuild) => {
			await updateWorkspaceBuild(build, queryClient);
		},
	};
};

export const deleteWorkspace = (
	workspace: Workspace,
	queryClient: QueryClient,
) => {
	return {
		mutationFn: (options: DeleteWorkspaceOptions) => {
			return API.deleteWorkspace(workspace.id, options);
		},
		onSuccess: async (build: WorkspaceBuild) => {
			await updateWorkspaceBuild(build, queryClient);
		},
	};
};

export const stopWorkspace = (
	workspace: Workspace,
	queryClient: QueryClient,
) => {
	return {
		mutationFn: ({ logLevel }: { logLevel?: ProvisionerLogLevel }) => {
			return API.stopWorkspace(workspace.id, logLevel);
		},
		onSuccess: async (build: WorkspaceBuild) => {
			await updateWorkspaceBuild(build, queryClient);
		},
	};
};

export const startWorkspace = (
	workspace: Workspace,
	queryClient: QueryClient,
) => {
	return {
		mutationFn: ({
			buildParameters,
			logLevel,
		}: {
			buildParameters?: WorkspaceBuildParameter[];
			logLevel?: ProvisionerLogLevel;
		}) => {
			return API.startWorkspace(
				workspace.id,
				workspace.latest_build.template_version_id,
				logLevel,
				buildParameters,
			);
		},
		onSuccess: async (build: WorkspaceBuild) => {
			await updateWorkspaceBuild(build, queryClient);
		},
	};
};

export const cancelBuild = (workspace: Workspace, queryClient: QueryClient) => {
	return {
		mutationFn: () => {
			const { status } = workspace.latest_build;
			const params =
				status === "pending" || status === "running"
					? { expect_status: status }
					: undefined;
			return API.cancelWorkspaceBuild(workspace.latest_build.id, params);
		},
		onSuccess: async () => {
			await queryClient.invalidateQueries({
				queryKey: workspaceBuildsKey(workspace.id),
			});
		},
	};
};

export const activate = (workspace: Workspace, queryClient: QueryClient) => {
	return {
		mutationFn: () => {
			return API.updateWorkspaceDormancy(workspace.id, false);
		},
		onSuccess: (updatedWorkspace: Workspace) => {
			queryClient.setQueryData(
				workspaceByOwnerAndNameKey(workspace.owner_name, workspace.name),
				updatedWorkspace,
			);
		},
	};
};

const updateWorkspaceBuild = async (
	build: WorkspaceBuild,
	queryClient: QueryClient,
) => {
	const workspaceKey = workspaceByOwnerAndNameKey(
		build.workspace_owner_name,
		build.workspace_name,
	);
	const previousData = queryClient.getQueryData<Workspace>(workspaceKey);
	if (!previousData) {
		return;
	}

	// Check if the build returned is newer than the previous build that could be
	// updated from web socket
	const previousUpdate = new Date(previousData.latest_build.updated_at);
	const newestUpdate = new Date(build.updated_at);
	if (newestUpdate > previousUpdate) {
		queryClient.setQueryData(workspaceKey, {
			...previousData,
			latest_build: build,
		});
	}

	await queryClient.invalidateQueries({
		queryKey: workspaceBuildsKey(build.workspace_id),
	});
};

export const toggleFavorite = (
	workspace: Workspace,
	queryClient: QueryClient,
) => {
	return {
		mutationFn: () => {
			if (workspace.favorite) {
				return API.deleteFavoriteWorkspace(workspace.id);
			}
			return API.putFavoriteWorkspace(workspace.id);
		},
		onSuccess: async () => {
			queryClient.setQueryData(
				workspaceByOwnerAndNameKey(workspace.owner_name, workspace.name),
				{ ...workspace, favorite: !workspace.favorite },
			);
			await queryClient.invalidateQueries({
				queryKey: workspaceByOwnerAndNameKey(
					workspace.owner_name,
					workspace.name,
				),
			});
		},
	};
};

export const buildLogsKey = (workspaceId: string) => [
	"workspaces",
	workspaceId,
	"logs",
];

export const buildLogs = (workspace: Workspace) => {
	return {
		queryKey: buildLogsKey(workspace.id),
		queryFn: () => API.getWorkspaceBuildLogs(workspace.latest_build.id),
	};
};

export const agentLogsKey = (agentId: string) => ["agents", agentId, "logs"];

export const agentLogs = (agentId: string) => {
	return {
		queryKey: agentLogsKey(agentId),
		queryFn: () => API.getWorkspaceAgentLogs(agentId),
		...disabledRefetchOptions,
	} satisfies UseQueryOptions<WorkspaceAgentLog[]>;
};

// workspace usage options
interface WorkspaceUsageOptions {
	usageApp: UsageAppName;
	connectionStatus: ConnectionStatus;
	workspaceId: string | undefined;
	agentId: string | undefined;
}

export const workspaceUsage = (options: WorkspaceUsageOptions) => {
	return {
		queryKey: [
			"workspaces",
			options.workspaceId,
			"agents",
			options.agentId,
			"usage",
			options.usageApp,
		],
		enabled:
			options.workspaceId !== undefined &&
			options.agentId !== undefined &&
			options.connectionStatus === "connected",
		queryFn: () => {
			if (options.workspaceId === undefined || options.agentId === undefined) {
				return Promise.reject();
			}

			return API.postWorkspaceUsage(options.workspaceId, {
				agent_id: options.agentId,
				app_name: options.usageApp,
			});
		},
		// ...disabledRefetchOptions,
		refetchInterval: 60000,
		refetchIntervalInBackground: true,
	};
};

export const workspacePermissions = (workspace?: Workspace) => {
	return {
		...checkAuthorization<WorkspacePermissions>({
			checks: workspace ? workspaceChecks(workspace) : {},
		}),
		queryKey: ["workspaces", workspace?.id, "permissions"],
		enabled: !!workspace,
		staleTime: Number.POSITIVE_INFINITY,
	};
};

export const workspaceAgentCredentials = (
	workspaceId: string,
	agentName: string,
) => {
	return {
		queryKey: ["workspaces", workspaceId, "agents", agentName, "credentials"],
		queryFn: () => API.getWorkspaceAgentCredentials(workspaceId, agentName),
	};
};
