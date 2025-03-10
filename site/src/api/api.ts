/**
 * @file Coder is starting to import the Coder API file into more and more
 * external projects, as a "pseudo-SDK". We are not at a stage where we are
 * ready to commit to maintaining a public SDK, but we need equivalent
 * functionality in other places.
 *
 * Message somebody from Team Blueberry if you need more context, but so far,
 * these projects are importing the file:
 *
 * - The Coder VS Code extension
 *   @see {@link https://github.com/coder/vscode-coder}
 * - The Coder Backstage plugin
 *   @see {@link https://github.com/coder/backstage-plugins}
 *
 * It is important that this file not do any aliased imports, or else the other
 * consumers could break (particularly for platforms that limit how much you can
 * touch their configuration files, like Backstage). Relative imports are still
 * safe, though.
 *
 * For example, `utils/delay` must be imported using `../utils/delay` instead.
 */
import globalAxios, { type AxiosInstance, isAxiosError } from "axios";
import type dayjs from "dayjs";
import userAgentParser from "ua-parser-js";
import { delay } from "../utils/delay";
import * as TypesGen from "./typesGenerated";
import type { PostWorkspaceUsageRequest } from "./typesGenerated";

const getMissingParameters = (
	oldBuildParameters: TypesGen.WorkspaceBuildParameter[],
	newBuildParameters: TypesGen.WorkspaceBuildParameter[],
	templateParameters: TypesGen.TemplateVersionParameter[],
) => {
	const missingParameters: TypesGen.TemplateVersionParameter[] = [];
	const requiredParameters: TypesGen.TemplateVersionParameter[] = [];

	for (const p of templateParameters) {
		// It is mutable and required. Mutable values can be changed after so we
		// don't need to ask them if they are not required.
		const isMutableAndRequired = p.mutable && p.required;
		// Is immutable, so we can check if it is its first time on the build
		const isImmutable = !p.mutable;

		if (isMutableAndRequired || isImmutable) {
			requiredParameters.push(p);
		}
	}

	for (const parameter of requiredParameters) {
		// Check if there is a new value
		let buildParameter = newBuildParameters.find(
			(p) => p.name === parameter.name,
		);

		// If not, get the old one
		if (!buildParameter) {
			buildParameter = oldBuildParameters.find(
				(p) => p.name === parameter.name,
			);
		}

		// If there is a value from the new or old one, it is not missed
		if (buildParameter) {
			continue;
		}

		missingParameters.push(parameter);
	}

	// Check if parameter "options" changed and we can't use old build parameters.
	for (const templateParameter of templateParameters) {
		if (templateParameter.options.length === 0) {
			continue;
		}

		// Check if there is a new value
		let buildParameter = newBuildParameters.find(
			(p) => p.name === templateParameter.name,
		);

		// If not, get the old one
		if (!buildParameter) {
			buildParameter = oldBuildParameters.find(
				(p) => p.name === templateParameter.name,
			);
		}

		if (!buildParameter) {
			continue;
		}

		const matchingOption = templateParameter.options.find(
			(option) => option.value === buildParameter?.value,
		);
		if (!matchingOption) {
			missingParameters.push(templateParameter);
		}
	}

	return missingParameters;
};

/**
 *
 * @param agentId
 * @returns An EventSource that emits agent metadata event objects
 * (ServerSentEvent)
 */
export const watchAgentMetadata = (agentId: string): EventSource => {
	return new EventSource(
		`${location.protocol}//${location.host}/api/v2/workspaceagents/${agentId}/watch-metadata`,
		{ withCredentials: true },
	);
};

/**
 * @returns {EventSource} An EventSource that emits workspace event objects
 * (ServerSentEvent)
 */
export const watchWorkspace = (workspaceId: string): EventSource => {
	return new EventSource(
		`${location.protocol}//${location.host}/api/v2/workspaces/${workspaceId}/watch`,
		{ withCredentials: true },
	);
};

export const getURLWithSearchParams = (
	basePath: string,
	options?: SearchParamOptions,
): string => {
	if (!options) {
		return basePath;
	}

	const searchParams = new URLSearchParams();
	for (const [key, value] of Object.entries(options)) {
		if (value !== undefined && value !== "") {
			searchParams.append(key, value.toString());
		}
	}

	const searchString = searchParams.toString();
	return searchString ? `${basePath}?${searchString}` : basePath;
};

// withDefaultFeatures sets all unspecified features to not_entitled and
// disabled.
export const withDefaultFeatures = (
	fs: Partial<TypesGen.Entitlements["features"]>,
): TypesGen.Entitlements["features"] => {
	for (const feature of TypesGen.FeatureNames) {
		// Skip fields that are already filled.
		if (fs[feature] !== undefined) {
			continue;
		}

		fs[feature] = {
			enabled: false,
			entitlement: "not_entitled",
		};
	}

	return fs as TypesGen.Entitlements["features"];
};

type WatchBuildLogsByTemplateVersionIdOptions = {
	after?: number;
	onMessage: (log: TypesGen.ProvisionerJobLog) => void;
	onDone?: () => void;
	onError: (error: Error) => void;
};

export const watchBuildLogsByTemplateVersionId = (
	versionId: string,
	{
		onMessage,
		onDone,
		onError,
		after,
	}: WatchBuildLogsByTemplateVersionIdOptions,
) => {
	const searchParams = new URLSearchParams({ follow: "true" });
	if (after !== undefined) {
		searchParams.append("after", after.toString());
	}

	const proto = location.protocol === "https:" ? "wss:" : "ws:";
	const socket = new WebSocket(
		`${proto}//${
			location.host
		}/api/v2/templateversions/${versionId}/logs?${searchParams.toString()}`,
	);

	socket.binaryType = "blob";

	socket.addEventListener("message", (event) =>
		onMessage(JSON.parse(event.data) as TypesGen.ProvisionerJobLog),
	);

	socket.addEventListener("error", () => {
		onError(new Error("Connection for logs failed."));
		socket.close();
	});

	socket.addEventListener("close", () => {
		// When the socket closes, logs have finished streaming!
		onDone?.();
	});

	return socket;
};

export const watchWorkspaceAgentLogs = (
	agentId: string,
	{ after, onMessage, onDone, onError }: WatchWorkspaceAgentLogsOptions,
) => {
	// WebSocket compression in Safari (confirmed in 16.5) is broken when
	// the server sends large messages. The following error is seen:
	//
	//   WebSocket connection to 'wss://.../logs?follow&after=0' failed: The operation couldn’t be completed. Protocol error
	//
	const noCompression =
		userAgentParser(navigator.userAgent).browser.name === "Safari"
			? "&no_compression"
			: "";

	const proto = location.protocol === "https:" ? "wss:" : "ws:";
	const socket = new WebSocket(
		`${proto}//${location.host}/api/v2/workspaceagents/${agentId}/logs?follow&after=${after}${noCompression}`,
	);
	socket.binaryType = "blob";

	socket.addEventListener("message", (event) => {
		const logs = JSON.parse(event.data) as TypesGen.WorkspaceAgentLog[];
		onMessage(logs);
	});

	socket.addEventListener("error", () => {
		onError(new Error("socket errored"));
	});

	socket.addEventListener("close", () => {
		onDone?.();
	});

	return socket;
};

type WatchWorkspaceAgentLogsOptions = {
	after: number;
	onMessage: (logs: TypesGen.WorkspaceAgentLog[]) => void;
	onDone?: () => void;
	onError: (error: Error) => void;
};

type WatchBuildLogsByBuildIdOptions = {
	after?: number;
	onMessage: (log: TypesGen.ProvisionerJobLog) => void;
	onDone?: () => void;
	onError?: (error: Error) => void;
};
export const watchBuildLogsByBuildId = (
	buildId: string,
	{ onMessage, onDone, onError, after }: WatchBuildLogsByBuildIdOptions,
) => {
	const searchParams = new URLSearchParams({ follow: "true" });
	if (after !== undefined) {
		searchParams.append("after", after.toString());
	}
	const proto = location.protocol === "https:" ? "wss:" : "ws:";
	const socket = new WebSocket(
		`${proto}//${
			location.host
		}/api/v2/workspacebuilds/${buildId}/logs?${searchParams.toString()}`,
	);
	socket.binaryType = "blob";

	socket.addEventListener("message", (event) =>
		onMessage(JSON.parse(event.data) as TypesGen.ProvisionerJobLog),
	);

	socket.addEventListener("error", () => {
		if (socket.readyState === socket.CLOSED) {
			return;
		}
		onError?.(new Error("Connection for logs failed."));
		socket.close();
	});

	socket.addEventListener("close", () => {
		// When the socket closes, logs have finished streaming!
		onDone?.();
	});

	return socket;
};

// This is the base header that is used for several requests. This is defined as
// a readonly value, but only copies of it should be passed into the API calls,
// because Axios is able to mutate the headers
const BASE_CONTENT_TYPE_JSON = {
	"Content-Type": "application/json",
} as const satisfies HeadersInit;

export type GetTemplatesOptions = Readonly<{
	readonly deprecated?: boolean;
}>;

export type GetTemplatesQuery = Readonly<{
	readonly q: string;
}>;

function normalizeGetTemplatesOptions(
	options: GetTemplatesOptions | GetTemplatesQuery = {},
): Record<string, string> {
	if ("q" in options) {
		return options;
	}

	const params: Record<string, string> = {};
	if (options.deprecated !== undefined) {
		params.deprecated = String(options.deprecated);
	}
	return params;
}

type SearchParamOptions = TypesGen.Pagination & {
	q?: string;
};

type RestartWorkspaceParameters = Readonly<{
	workspace: TypesGen.Workspace;
	buildParameters?: TypesGen.WorkspaceBuildParameter[];
}>;

export type DeleteWorkspaceOptions = Pick<
	TypesGen.CreateWorkspaceBuildRequest,
	"log_level" | "orphan"
>;

export type DeploymentConfig = Readonly<{
	config: TypesGen.DeploymentValues;
	options: TypesGen.SerpentOption[];
}>;

type Claims = {
	license_expires: number;
	account_type?: string;
	account_id?: string;
	trial: boolean;
	all_features: boolean;
	// feature_set is omitted on legacy licenses
	feature_set?: string;
	version: number;
	features: Record<string, number>;
	require_telemetry?: boolean;
};

export type GetLicensesResponse = Omit<TypesGen.License, "claims"> & {
	claims: Claims;
	expires_at: string;
};

export type InsightsParams = {
	start_time: string;
	end_time: string;
	template_ids: string;
};

export type InsightsTemplateParams = InsightsParams & {
	interval: "day" | "week";
};

export type GetJFrogXRayScanParams = {
	workspaceId: string;
	agentId: string;
};

export class MissingBuildParameters extends Error {
	parameters: TypesGen.TemplateVersionParameter[] = [];
	versionId: string;

	constructor(
		parameters: TypesGen.TemplateVersionParameter[],
		versionId: string,
	) {
		super("Missing build parameters.");
		this.parameters = parameters;
		this.versionId = versionId;
	}
}

/**
 * This is the container for all API methods. It's split off to make it more
 * clear where API methods should go, but it is eventually merged into the Api
 * class with a more flat hierarchy
 *
 * All public methods should be defined as arrow functions to ensure that they
 * can be passed around the React UI without losing their `this` context.
 *
 * This is one of the few cases where you have to worry about the difference
 * between traditional methods and arrow function properties. Arrow functions
 * disable JS's dynamic scope, and force all `this` references to resolve via
 * lexical scope.
 */
class ApiMethods {
	constructor(protected readonly axios: AxiosInstance) {}

	login = async (
		email: string,
		password: string,
	): Promise<TypesGen.LoginWithPasswordResponse> => {
		const payload = JSON.stringify({ email, password });
		const response = await this.axios.post<TypesGen.LoginWithPasswordResponse>(
			"/api/v2/users/login",
			payload,
			{ headers: { ...BASE_CONTENT_TYPE_JSON } },
		);

		return response.data;
	};

	convertToOAUTH = async (request: TypesGen.ConvertLoginRequest) => {
		const response = await this.axios.post<TypesGen.OAuthConversionResponse>(
			"/api/v2/users/me/convert-login",
			request,
		);

		return response.data;
	};

	logout = async (): Promise<void> => {
		return this.axios.post("/api/v2/users/logout");
	};

	getAuthenticatedUser = async () => {
		const response = await this.axios.get<TypesGen.User>("/api/v2/users/me");
		return response.data;
	};

	getUserParameters = async (templateID: string) => {
		const response = await this.axios.get<TypesGen.UserParameter[]>(
			`/api/v2/users/me/autofill-parameters?template_id=${templateID}`,
		);

		return response.data;
	};

	getAuthMethods = async (): Promise<TypesGen.AuthMethods> => {
		const response = await this.axios.get<TypesGen.AuthMethods>(
			"/api/v2/users/authmethods",
		);

		return response.data;
	};

	getUserLoginType = async (): Promise<TypesGen.UserLoginType> => {
		const response = await this.axios.get<TypesGen.UserLoginType>(
			"/api/v2/users/me/login-type",
		);

		return response.data;
	};

	checkAuthorization = async (
		params: TypesGen.AuthorizationRequest,
	): Promise<TypesGen.AuthorizationResponse> => {
		const response = await this.axios.post<TypesGen.AuthorizationResponse>(
			"/api/v2/authcheck",
			params,
		);

		return response.data;
	};

	getApiKey = async (): Promise<TypesGen.GenerateAPIKeyResponse> => {
		const response = await this.axios.post<TypesGen.GenerateAPIKeyResponse>(
			"/api/v2/users/me/keys",
		);

		return response.data;
	};

	getTokens = async (
		params: TypesGen.TokensFilter,
	): Promise<TypesGen.APIKeyWithOwner[]> => {
		const response = await this.axios.get<TypesGen.APIKeyWithOwner[]>(
			"/api/v2/users/me/keys/tokens",
			{ params },
		);

		return response.data;
	};

	deleteToken = async (keyId: string): Promise<void> => {
		await this.axios.delete(`/api/v2/users/me/keys/${keyId}`);
	};

	createToken = async (
		params: TypesGen.CreateTokenRequest,
	): Promise<TypesGen.GenerateAPIKeyResponse> => {
		const response = await this.axios.post(
			"/api/v2/users/me/keys/tokens",
			params,
		);

		return response.data;
	};

	getTokenConfig = async (): Promise<TypesGen.TokenConfig> => {
		const response = await this.axios.get(
			"/api/v2/users/me/keys/tokens/tokenconfig",
		);

		return response.data;
	};

	getUsers = async (
		options: TypesGen.UsersRequest,
		signal?: AbortSignal,
	): Promise<TypesGen.GetUsersResponse> => {
		const url = getURLWithSearchParams("/api/v2/users", options);
		const response = await this.axios.get<TypesGen.GetUsersResponse>(
			url.toString(),
			{ signal },
		);

		return response.data;
	};

	createOrganization = async (params: TypesGen.CreateOrganizationRequest) => {
		const response = await this.axios.post<TypesGen.Organization>(
			"/api/v2/organizations",
			params,
		);
		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	updateOrganization = async (
		organization: string,
		params: TypesGen.UpdateOrganizationRequest,
	) => {
		const response = await this.axios.patch<TypesGen.Organization>(
			`/api/v2/organizations/${organization}`,
			params,
		);
		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	deleteOrganization = async (organization: string) => {
		await this.axios.delete<TypesGen.Organization>(
			`/api/v2/organizations/${organization}`,
		);
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	getOrganization = async (
		organization: string,
	): Promise<TypesGen.Organization> => {
		const response = await this.axios.get<TypesGen.Organization>(
			`/api/v2/organizations/${organization}`,
		);

		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	getOrganizationMembers = async (organization: string) => {
		const response = await this.axios.get<
			TypesGen.OrganizationMemberWithUserData[]
		>(`/api/v2/organizations/${organization}/members`);

		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	getOrganizationRoles = async (organization: string) => {
		const response = await this.axios.get<TypesGen.AssignableRoles[]>(
			`/api/v2/organizations/${organization}/members/roles`,
		);

		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	updateOrganizationMemberRoles = async (
		organization: string,
		userId: string,
		roles: TypesGen.SlimRole["name"][],
	): Promise<TypesGen.User> => {
		const response = await this.axios.put<TypesGen.User>(
			`/api/v2/organizations/${organization}/members/${userId}/roles`,
			{ roles },
		);

		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	createOrganizationRole = async (
		organization: string,
		role: TypesGen.Role,
	): Promise<TypesGen.Role> => {
		const response = await this.axios.post<TypesGen.Role>(
			`/api/v2/organizations/${organization}/members/roles`,
			role,
		);

		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	updateOrganizationRole = async (
		organization: string,
		role: TypesGen.Role,
	): Promise<TypesGen.Role> => {
		const response = await this.axios.put<TypesGen.Role>(
			`/api/v2/organizations/${organization}/members/roles`,
			role,
		);

		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	deleteOrganizationRole = async (organization: string, roleName: string) => {
		await this.axios.delete(
			`/api/v2/organizations/${organization}/members/roles/${roleName}`,
		);
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	addOrganizationMember = async (organization: string, userId: string) => {
		const response = await this.axios.post<TypesGen.OrganizationMember>(
			`/api/v2/organizations/${organization}/members/${userId}`,
		);

		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	removeOrganizationMember = async (organization: string, userId: string) => {
		await this.axios.delete(
			`/api/v2/organizations/${organization}/members/${userId}`,
		);
	};

	getOrganizations = async (): Promise<TypesGen.Organization[]> => {
		const response = await this.axios.get<TypesGen.Organization[]>(
			"/api/v2/organizations",
		);
		return response.data;
	};

	getMyOrganizations = async (): Promise<TypesGen.Organization[]> => {
		const response = await this.axios.get<TypesGen.Organization[]>(
			"/api/v2/users/me/organizations",
		);
		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 * @param tags to filter provisioner daemons by.
	 */
	getProvisionerDaemonsByOrganization = async (
		organization: string,
		tags?: Record<string, string>,
	): Promise<TypesGen.ProvisionerDaemon[]> => {
		const params = new URLSearchParams();

		if (tags) {
			params.append("tags", JSON.stringify(tags));
		}

		const response = await this.axios.get<TypesGen.ProvisionerDaemon[]>(
			`/api/v2/organizations/${organization}/provisionerdaemons?${params}`,
		);
		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	getProvisionerDaemonGroupsByOrganization = async (
		organization: string,
	): Promise<TypesGen.ProvisionerKeyDaemons[]> => {
		const response = await this.axios.get<TypesGen.ProvisionerKeyDaemons[]>(
			`/api/v2/organizations/${organization}/provisionerkeys/daemons`,
		);
		return response.data;
	};

	getOrganizationIdpSyncSettings =
		async (): Promise<TypesGen.OrganizationSyncSettings> => {
			const response = await this.axios.get<TypesGen.OrganizationSyncSettings>(
				"/api/v2/settings/idpsync/organization",
			);
			return response.data;
		};

	patchOrganizationIdpSyncSettings = async (
		data: TypesGen.OrganizationSyncSettings,
	) => {
		const response = await this.axios.patch<TypesGen.Response>(
			"/api/v2/settings/idpsync/organization",
			data,
		);
		return response.data;
	};

	/**
	 * @param data
	 * @param organization Can be the organization's ID or name
	 */
	patchGroupIdpSyncSettings = async (
		data: TypesGen.GroupSyncSettings,
		organization: string,
	) => {
		const response = await this.axios.patch<TypesGen.Response>(
			`/api/v2/organizations/${organization}/settings/idpsync/groups`,
			data,
		);
		return response.data;
	};

	/**
	 * @param data
	 * @param organization Can be the organization's ID or name
	 */
	patchRoleIdpSyncSettings = async (
		data: TypesGen.RoleSyncSettings,
		organization: string,
	) => {
		const response = await this.axios.patch<TypesGen.Response>(
			`/api/v2/organizations/${organization}/settings/idpsync/roles`,
			data,
		);
		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	getGroupIdpSyncSettingsByOrganization = async (
		organization: string,
	): Promise<TypesGen.GroupSyncSettings> => {
		const response = await this.axios.get<TypesGen.GroupSyncSettings>(
			`/api/v2/organizations/${organization}/settings/idpsync/groups`,
		);
		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	getRoleIdpSyncSettingsByOrganization = async (
		organization: string,
	): Promise<TypesGen.RoleSyncSettings> => {
		const response = await this.axios.get<TypesGen.RoleSyncSettings>(
			`/api/v2/organizations/${organization}/settings/idpsync/roles`,
		);
		return response.data;
	};

	getDeploymentIdpSyncFieldValues = async (
		field: string,
	): Promise<readonly string[]> => {
		const params = new URLSearchParams();
		params.set("claimField", field);
		const response = await this.axios.get<readonly string[]>(
			`/api/v2/settings/idpsync/field-values?${params}`,
		);
		return response.data;
	};

	getOrganizationIdpSyncClaimFieldValues = async (
		organization: string,
		field: string,
	) => {
		const params = new URLSearchParams();
		params.set("claimField", field);
		const response = await this.axios.get<readonly string[]>(
			`/api/v2/organizations/${organization}/settings/idpsync/field-values?${params}`,
		);
		return response.data;
	};

	getTemplate = async (templateId: string): Promise<TypesGen.Template> => {
		const response = await this.axios.get<TypesGen.Template>(
			`/api/v2/templates/${templateId}`,
		);

		return response.data;
	};

	getTemplates = async (
		options?: GetTemplatesOptions | GetTemplatesQuery,
	): Promise<TypesGen.Template[]> => {
		const params = normalizeGetTemplatesOptions(options);
		const response = await this.axios.get<TypesGen.Template[]>(
			"/api/v2/templates",
			{ params },
		);

		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	getTemplatesByOrganization = async (
		organization: string,
		options?: GetTemplatesOptions,
	): Promise<TypesGen.Template[]> => {
		const params = normalizeGetTemplatesOptions(options);
		const response = await this.axios.get<TypesGen.Template[]>(
			`/api/v2/organizations/${organization}/templates`,
			{ params },
		);

		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	getTemplateByName = async (
		organization: string,
		name: string,
	): Promise<TypesGen.Template> => {
		const response = await this.axios.get<TypesGen.Template>(
			`/api/v2/organizations/${organization}/templates/${name}`,
		);

		return response.data;
	};

	getTemplateVersion = async (
		versionId: string,
	): Promise<TypesGen.TemplateVersion> => {
		const response = await this.axios.get<TypesGen.TemplateVersion>(
			`/api/v2/templateversions/${versionId}`,
		);

		return response.data;
	};

	getTemplateVersionResources = async (
		versionId: string,
	): Promise<TypesGen.WorkspaceResource[]> => {
		const response = await this.axios.get<TypesGen.WorkspaceResource[]>(
			`/api/v2/templateversions/${versionId}/resources`,
		);

		return response.data;
	};

	getTemplateVersionVariables = async (
		versionId: string,
	): Promise<TypesGen.TemplateVersionVariable[]> => {
		// Defined as separate variable to avoid wonky Prettier formatting because
		// the type definition is so long
		type VerArray = TypesGen.TemplateVersionVariable[];

		const response = await this.axios.get<VerArray>(
			`/api/v2/templateversions/${versionId}/variables`,
		);

		return response.data;
	};

	getTemplateVersions = async (
		templateId: string,
	): Promise<TypesGen.TemplateVersion[]> => {
		const response = await this.axios.get<TypesGen.TemplateVersion[]>(
			`/api/v2/templates/${templateId}/versions`,
		);
		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	getTemplateVersionByName = async (
		organization: string,
		templateName: string,
		versionName: string,
	): Promise<TypesGen.TemplateVersion> => {
		const response = await this.axios.get<TypesGen.TemplateVersion>(
			`/api/v2/organizations/${organization}/templates/${templateName}/versions/${versionName}`,
		);

		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	getPreviousTemplateVersionByName = async (
		organization: string,
		templateName: string,
		versionName: string,
	) => {
		try {
			const response = await this.axios.get<TypesGen.TemplateVersion>(
				`/api/v2/organizations/${organization}/templates/${templateName}/versions/${versionName}/previous`,
			);

			return response.data;
		} catch (error) {
			// When there is no previous version, like the first version of a
			// template, the API returns 404 so in this case we can safely return
			// undefined
			const is404 =
				isAxiosError(error) && error.response && error.response.status === 404;

			if (is404) {
				return undefined;
			}

			throw error;
		}
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	createTemplateVersion = async (
		organization: string,
		data: TypesGen.CreateTemplateVersionRequest,
	): Promise<TypesGen.TemplateVersion> => {
		const response = await this.axios.post<TypesGen.TemplateVersion>(
			`/api/v2/organizations/${organization}/templateversions`,
			data,
		);

		return response.data;
	};

	getTemplateVersionExternalAuth = async (
		versionId: string,
	): Promise<TypesGen.TemplateVersionExternalAuth[]> => {
		const response = await this.axios.get(
			`/api/v2/templateversions/${versionId}/external-auth`,
		);

		return response.data;
	};

	getTemplateVersionRichParameters = async (
		versionId: string,
	): Promise<TypesGen.TemplateVersionParameter[]> => {
		const response = await this.axios.get(
			`/api/v2/templateversions/${versionId}/rich-parameters`,
		);
		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	createTemplate = async (
		organization: string,
		data: TypesGen.CreateTemplateRequest,
	): Promise<TypesGen.Template> => {
		const response = await this.axios.post(
			`/api/v2/organizations/${organization}/templates`,
			data,
		);

		return response.data;
	};

	updateActiveTemplateVersion = async (
		templateId: string,
		data: TypesGen.UpdateActiveTemplateVersion,
	) => {
		const response = await this.axios.patch<TypesGen.Response>(
			`/api/v2/templates/${templateId}/versions`,
			data,
		);
		return response.data;
	};

	patchTemplateVersion = async (
		templateVersionId: string,
		data: TypesGen.PatchTemplateVersionRequest,
	) => {
		const response = await this.axios.patch<TypesGen.TemplateVersion>(
			`/api/v2/templateversions/${templateVersionId}`,
			data,
		);

		return response.data;
	};

	archiveTemplateVersion = async (templateVersionId: string) => {
		const response = await this.axios.post<TypesGen.TemplateVersion>(
			`/api/v2/templateversions/${templateVersionId}/archive`,
		);

		return response.data;
	};

	unarchiveTemplateVersion = async (templateVersionId: string) => {
		const response = await this.axios.post<TypesGen.TemplateVersion>(
			`/api/v2/templateversions/${templateVersionId}/unarchive`,
		);
		return response.data;
	};

	updateTemplateMeta = async (
		templateId: string,
		data: TypesGen.UpdateTemplateMeta,
	): Promise<TypesGen.Template | null> => {
		const response = await this.axios.patch<TypesGen.Template>(
			`/api/v2/templates/${templateId}`,
			data,
		);

		// On 304 response there is no data payload.
		if (response.status === 304) {
			return null;
		}

		return response.data;
	};

	deleteTemplate = async (templateId: string): Promise<TypesGen.Template> => {
		const response = await this.axios.delete<TypesGen.Template>(
			`/api/v2/templates/${templateId}`,
		);

		return response.data;
	};

	getWorkspace = async (
		workspaceId: string,
		params?: TypesGen.WorkspaceOptions,
	): Promise<TypesGen.Workspace> => {
		const response = await this.axios.get<TypesGen.Workspace>(
			`/api/v2/workspaces/${workspaceId}`,
			{ params },
		);

		return response.data;
	};

	getWorkspaces = async (
		options: TypesGen.WorkspacesRequest,
	): Promise<TypesGen.WorkspacesResponse> => {
		const url = getURLWithSearchParams("/api/v2/workspaces", options);
		const response = await this.axios.get<TypesGen.WorkspacesResponse>(url);
		return response.data;
	};

	getWorkspaceByOwnerAndName = async (
		username = "me",
		workspaceName: string,
		params?: TypesGen.WorkspaceOptions,
	): Promise<TypesGen.Workspace> => {
		const response = await this.axios.get<TypesGen.Workspace>(
			`/api/v2/users/${username}/workspace/${workspaceName}`,
			{ params },
		);

		return response.data;
	};

	getWorkspaceBuildByNumber = async (
		username = "me",
		workspaceName: string,
		buildNumber: number,
	): Promise<TypesGen.WorkspaceBuild> => {
		const response = await this.axios.get<TypesGen.WorkspaceBuild>(
			`/api/v2/users/${username}/workspace/${workspaceName}/builds/${buildNumber}`,
		);

		return response.data;
	};

	waitForBuild = (build: TypesGen.WorkspaceBuild) => {
		return new Promise<TypesGen.ProvisionerJob | undefined>((res, reject) => {
			void (async () => {
				let latestJobInfo: TypesGen.ProvisionerJob | undefined = undefined;

				while (
					!["succeeded", "canceled"].some((status) =>
						latestJobInfo?.status.includes(status),
					)
				) {
					const { job } = await this.getWorkspaceBuildByNumber(
						build.workspace_owner_name,
						build.workspace_name,
						build.build_number,
					);

					latestJobInfo = job;
					if (latestJobInfo.status === "failed") {
						return reject(latestJobInfo);
					}

					await delay(1000);
				}

				return res(latestJobInfo);
			})();
		});
	};

	postWorkspaceBuild = async (
		workspaceId: string,
		data: TypesGen.CreateWorkspaceBuildRequest,
	): Promise<TypesGen.WorkspaceBuild> => {
		const response = await this.axios.post(
			`/api/v2/workspaces/${workspaceId}/builds`,
			data,
		);

		return response.data;
	};

	getTemplateVersionPresets = async (
		templateVersionId: string,
	): Promise<TypesGen.Preset[]> => {
		const response = await this.axios.get<TypesGen.Preset[]>(
			`/api/v2/templateversions/${templateVersionId}/presets`,
		);
		return response.data;
	};

	startWorkspace = (
		workspaceId: string,
		templateVersionId: string,
		logLevel?: TypesGen.ProvisionerLogLevel,
		buildParameters?: TypesGen.WorkspaceBuildParameter[],
	) => {
		return this.postWorkspaceBuild(workspaceId, {
			transition: "start",
			template_version_id: templateVersionId,
			log_level: logLevel,
			rich_parameter_values: buildParameters,
		});
	};

	stopWorkspace = (
		workspaceId: string,
		logLevel?: TypesGen.ProvisionerLogLevel,
	) => {
		return this.postWorkspaceBuild(workspaceId, {
			transition: "stop",
			log_level: logLevel,
		});
	};

	deleteWorkspace = (workspaceId: string, options?: DeleteWorkspaceOptions) => {
		return this.postWorkspaceBuild(workspaceId, {
			transition: "delete",
			...options,
		});
	};

	cancelWorkspaceBuild = async (
		workspaceBuildId: TypesGen.WorkspaceBuild["id"],
	): Promise<TypesGen.Response> => {
		const response = await this.axios.patch(
			`/api/v2/workspacebuilds/${workspaceBuildId}/cancel`,
		);

		return response.data;
	};

	updateWorkspaceDormancy = async (
		workspaceId: string,
		dormant: boolean,
	): Promise<TypesGen.Workspace> => {
		const data: TypesGen.UpdateWorkspaceDormancy = { dormant };
		const response = await this.axios.put(
			`/api/v2/workspaces/${workspaceId}/dormant`,
			data,
		);

		return response.data;
	};

	updateWorkspaceAutomaticUpdates = async (
		workspaceId: string,
		automaticUpdates: TypesGen.AutomaticUpdates,
	): Promise<void> => {
		const req: TypesGen.UpdateWorkspaceAutomaticUpdatesRequest = {
			automatic_updates: automaticUpdates,
		};

		const response = await this.axios.put(
			`/api/v2/workspaces/${workspaceId}/autoupdates`,
			req,
		);

		return response.data;
	};

	restartWorkspace = async ({
		workspace,
		buildParameters,
	}: RestartWorkspaceParameters): Promise<void> => {
		const stopBuild = await this.stopWorkspace(workspace.id);
		const awaitedStopBuild = await this.waitForBuild(stopBuild);

		// If the restart is canceled halfway through, make sure we bail
		if (awaitedStopBuild?.status === "canceled") {
			return;
		}

		const startBuild = await this.startWorkspace(
			workspace.id,
			workspace.latest_build.template_version_id,
			undefined,
			buildParameters,
		);

		await this.waitForBuild(startBuild);
	};

	cancelTemplateVersionBuild = async (
		templateVersionId: string,
	): Promise<TypesGen.Response> => {
		const response = await this.axios.patch(
			`/api/v2/templateversions/${templateVersionId}/cancel`,
		);

		return response.data;
	};

	cancelTemplateVersionDryRun = async (
		templateVersionId: string,
		jobId: string,
	): Promise<TypesGen.Response> => {
		const response = await this.axios.patch(
			`/api/v2/templateversions/${templateVersionId}/dry-run/${jobId}/cancel`,
		);

		return response.data;
	};

	createUser = async (
		user: TypesGen.CreateUserRequestWithOrgs,
	): Promise<TypesGen.User> => {
		const response = await this.axios.post<TypesGen.User>(
			"/api/v2/users",
			user,
		);

		return response.data;
	};

	createWorkspace = async (
		userId = "me",
		workspace: TypesGen.CreateWorkspaceRequest,
	): Promise<TypesGen.Workspace> => {
		const response = await this.axios.post<TypesGen.Workspace>(
			`/api/v2/users/${userId}/workspaces`,
			workspace,
		);

		return response.data;
	};

	patchWorkspace = async (
		workspaceId: string,
		data: TypesGen.UpdateWorkspaceRequest,
	): Promise<void> => {
		await this.axios.patch(`/api/v2/workspaces/${workspaceId}`, data);
	};

	getBuildInfo = async (): Promise<TypesGen.BuildInfoResponse> => {
		const response = await this.axios.get("/api/v2/buildinfo");
		return response.data;
	};

	getUpdateCheck = async (): Promise<TypesGen.UpdateCheckResponse> => {
		const response = await this.axios.get("/api/v2/updatecheck");
		return response.data;
	};

	putWorkspaceAutostart = async (
		workspaceID: string,
		autostart: TypesGen.UpdateWorkspaceAutostartRequest,
	): Promise<void> => {
		const payload = JSON.stringify(autostart);
		await this.axios.put(
			`/api/v2/workspaces/${workspaceID}/autostart`,
			payload,
			{ headers: { ...BASE_CONTENT_TYPE_JSON } },
		);
	};

	putWorkspaceAutostop = async (
		workspaceID: string,
		ttl: TypesGen.UpdateWorkspaceTTLRequest,
	): Promise<void> => {
		const payload = JSON.stringify(ttl);
		await this.axios.put(`/api/v2/workspaces/${workspaceID}/ttl`, payload, {
			headers: { ...BASE_CONTENT_TYPE_JSON },
		});
	};

	updateProfile = async (
		userId: string,
		data: TypesGen.UpdateUserProfileRequest,
	): Promise<TypesGen.User> => {
		const response = await this.axios.put(
			`/api/v2/users/${userId}/profile`,
			data,
		);
		return response.data;
	};

	getAppearanceSettings =
		async (): Promise<TypesGen.UserAppearanceSettings> => {
			const response = await this.axios.get("/api/v2/users/me/appearance");
			return response.data;
		};

	updateAppearanceSettings = async (
		data: TypesGen.UpdateUserAppearanceSettingsRequest,
	): Promise<TypesGen.UserAppearanceSettings> => {
		const response = await this.axios.put("/api/v2/users/me/appearance", data);
		return response.data;
	};

	getUserQuietHoursSchedule = async (
		userId: TypesGen.User["id"],
	): Promise<TypesGen.UserQuietHoursScheduleResponse> => {
		const response = await this.axios.get(
			`/api/v2/users/${userId}/quiet-hours`,
		);
		return response.data;
	};

	updateUserQuietHoursSchedule = async (
		userId: TypesGen.User["id"],
		data: TypesGen.UpdateUserQuietHoursScheduleRequest,
	): Promise<TypesGen.UserQuietHoursScheduleResponse> => {
		const response = await this.axios.put(
			`/api/v2/users/${userId}/quiet-hours`,
			data,
		);

		return response.data;
	};

	activateUser = async (
		userId: TypesGen.User["id"],
	): Promise<TypesGen.User> => {
		const response = await this.axios.put<TypesGen.User>(
			`/api/v2/users/${userId}/status/activate`,
		);
		return response.data;
	};

	suspendUser = async (userId: TypesGen.User["id"]): Promise<TypesGen.User> => {
		const response = await this.axios.put<TypesGen.User>(
			`/api/v2/users/${userId}/status/suspend`,
		);

		return response.data;
	};

	deleteUser = async (userId: TypesGen.User["id"]): Promise<void> => {
		await this.axios.delete(`/api/v2/users/${userId}`);
	};

	// API definition:
	// https://github.com/coder/coder/blob/db665e7261f3c24a272ccec48233a3e276878239/coderd/users.go#L33-L53
	hasFirstUser = async (): Promise<boolean> => {
		try {
			// If it is success, it is true
			await this.axios.get("/api/v2/users/first");
			return true;
		} catch (error) {
			// If it returns a 404, it is false
			if (isAxiosError(error) && error.response?.status === 404) {
				return false;
			}

			throw error;
		}
	};

	createFirstUser = async (
		req: TypesGen.CreateFirstUserRequest,
	): Promise<TypesGen.CreateFirstUserResponse> => {
		const response = await this.axios.post("/api/v2/users/first", req);
		return response.data;
	};

	updateUserPassword = async (
		userId: TypesGen.User["id"],
		updatePassword: TypesGen.UpdateUserPasswordRequest,
	): Promise<void> => {
		await this.axios.put(`/api/v2/users/${userId}/password`, updatePassword);
	};

	validateUserPassword = async (
		password: string,
	): Promise<TypesGen.ValidateUserPasswordResponse> => {
		const response = await this.axios.post("/api/v2/users/validate-password", {
			password,
		});
		return response.data;
	};

	getRoles = async (): Promise<Array<TypesGen.AssignableRoles>> => {
		const response = await this.axios.get<TypesGen.AssignableRoles[]>(
			"/api/v2/users/roles",
		);

		return response.data;
	};

	updateUserRoles = async (
		roles: TypesGen.SlimRole["name"][],
		userId: TypesGen.User["id"],
	): Promise<TypesGen.User> => {
		const response = await this.axios.put<TypesGen.User>(
			`/api/v2/users/${userId}/roles`,
			{ roles },
		);

		return response.data;
	};

	getUserSSHKey = async (userId = "me"): Promise<TypesGen.GitSSHKey> => {
		const response = await this.axios.get<TypesGen.GitSSHKey>(
			`/api/v2/users/${userId}/gitsshkey`,
		);

		return response.data;
	};

	regenerateUserSSHKey = async (userId = "me"): Promise<TypesGen.GitSSHKey> => {
		const response = await this.axios.put<TypesGen.GitSSHKey>(
			`/api/v2/users/${userId}/gitsshkey`,
		);

		return response.data;
	};

	getWorkspaceBuilds = async (
		workspaceId: string,
		req?: TypesGen.WorkspaceBuildsRequest,
	) => {
		const response = await this.axios.get<TypesGen.WorkspaceBuild[]>(
			getURLWithSearchParams(`/api/v2/workspaces/${workspaceId}/builds`, req),
		);

		return response.data;
	};

	getWorkspaceBuildLogs = async (
		buildId: string,
	): Promise<TypesGen.ProvisionerJobLog[]> => {
		const response = await this.axios.get<TypesGen.ProvisionerJobLog[]>(
			`/api/v2/workspacebuilds/${buildId}/logs`,
		);

		return response.data;
	};

	getWorkspaceAgentLogs = async (
		agentID: string,
	): Promise<TypesGen.WorkspaceAgentLog[]> => {
		const response = await this.axios.get<TypesGen.WorkspaceAgentLog[]>(
			`/api/v2/workspaceagents/${agentID}/logs`,
		);

		return response.data;
	};

	putWorkspaceExtension = async (
		workspaceId: string,
		newDeadline: dayjs.Dayjs,
	): Promise<void> => {
		await this.axios.put(`/api/v2/workspaces/${workspaceId}/extend`, {
			deadline: newDeadline,
		});
	};

	refreshEntitlements = async (): Promise<void> => {
		await this.axios.post("/api/v2/licenses/refresh-entitlements");
	};

	getEntitlements = async (): Promise<TypesGen.Entitlements> => {
		try {
			const response = await this.axios.get<TypesGen.Entitlements>(
				"/api/v2/entitlements",
			);

			return response.data;
		} catch (ex) {
			if (isAxiosError(ex) && ex.response?.status === 404) {
				return {
					errors: [],
					features: withDefaultFeatures({}),
					has_license: false,
					require_telemetry: false,
					trial: false,
					warnings: [],
					refreshed_at: "",
				};
			}
			throw ex;
		}
	};

	getExperiments = async (): Promise<TypesGen.Experiment[]> => {
		try {
			const response = await this.axios.get<TypesGen.Experiment[]>(
				"/api/v2/experiments",
			);

			return response.data;
		} catch (error) {
			if (isAxiosError(error) && error.response?.status === 404) {
				return [];
			}

			throw error;
		}
	};

	getAvailableExperiments =
		async (): Promise<TypesGen.AvailableExperiments> => {
			try {
				const response = await this.axios.get("/api/v2/experiments/available");

				return response.data;
			} catch (error) {
				if (isAxiosError(error) && error.response?.status === 404) {
					return { safe: [] };
				}
				throw error;
			}
		};

	getExternalAuthProvider = async (
		provider: string,
	): Promise<TypesGen.ExternalAuth> => {
		const res = await this.axios.get(`/api/v2/external-auth/${provider}`);
		return res.data;
	};

	getExternalAuthDevice = async (
		provider: string,
	): Promise<TypesGen.ExternalAuthDevice> => {
		const resp = await this.axios.get(
			`/api/v2/external-auth/${provider}/device`,
		);
		return resp.data;
	};

	exchangeExternalAuthDevice = async (
		provider: string,
		req: TypesGen.ExternalAuthDeviceExchange,
	): Promise<void> => {
		const resp = await this.axios.post(
			`/api/v2/external-auth/${provider}/device`,
			req,
		);

		return resp.data;
	};

	getUserExternalAuthProviders =
		async (): Promise<TypesGen.ListUserExternalAuthResponse> => {
			const resp = await this.axios.get("/api/v2/external-auth");
			return resp.data;
		};

	unlinkExternalAuthProvider = async (provider: string): Promise<string> => {
		const resp = await this.axios.delete(`/api/v2/external-auth/${provider}`);
		return resp.data;
	};

	getOAuth2GitHubDeviceFlowCallback = async (
		code: string,
		state: string,
	): Promise<TypesGen.OAuth2DeviceFlowCallbackResponse> => {
		const resp = await this.axios.get(
			`/api/v2/users/oauth2/github/callback?code=${code}&state=${state}`,
		);
		// sanity check
		if (
			typeof resp.data !== "object" ||
			typeof resp.data.redirect_url !== "string"
		) {
			console.error("Invalid response from OAuth2 GitHub callback", resp);
			throw new Error("Invalid response from OAuth2 GitHub callback");
		}
		return resp.data;
	};

	getOAuth2GitHubDevice = async (): Promise<TypesGen.ExternalAuthDevice> => {
		const resp = await this.axios.get("/api/v2/users/oauth2/github/device");
		return resp.data;
	};

	getOAuth2ProviderApps = async (
		filter?: TypesGen.OAuth2ProviderAppFilter,
	): Promise<TypesGen.OAuth2ProviderApp[]> => {
		const params = filter?.user_id
			? new URLSearchParams({ user_id: filter.user_id }).toString()
			: "";

		const resp = await this.axios.get(`/api/v2/oauth2-provider/apps?${params}`);
		return resp.data;
	};

	getOAuth2ProviderApp = async (
		id: string,
	): Promise<TypesGen.OAuth2ProviderApp> => {
		const resp = await this.axios.get(`/api/v2/oauth2-provider/apps/${id}`);
		return resp.data;
	};

	postOAuth2ProviderApp = async (
		data: TypesGen.PostOAuth2ProviderAppRequest,
	): Promise<TypesGen.OAuth2ProviderApp> => {
		const response = await this.axios.post(
			"/api/v2/oauth2-provider/apps",
			data,
		);
		return response.data;
	};

	putOAuth2ProviderApp = async (
		id: string,
		data: TypesGen.PutOAuth2ProviderAppRequest,
	): Promise<TypesGen.OAuth2ProviderApp> => {
		const response = await this.axios.put(
			`/api/v2/oauth2-provider/apps/${id}`,
			data,
		);
		return response.data;
	};

	deleteOAuth2ProviderApp = async (id: string): Promise<void> => {
		await this.axios.delete(`/api/v2/oauth2-provider/apps/${id}`);
	};

	getOAuth2ProviderAppSecrets = async (
		id: string,
	): Promise<TypesGen.OAuth2ProviderAppSecret[]> => {
		const resp = await this.axios.get(
			`/api/v2/oauth2-provider/apps/${id}/secrets`,
		);
		return resp.data;
	};

	postOAuth2ProviderAppSecret = async (
		id: string,
	): Promise<TypesGen.OAuth2ProviderAppSecretFull> => {
		const resp = await this.axios.post(
			`/api/v2/oauth2-provider/apps/${id}/secrets`,
		);
		return resp.data;
	};

	deleteOAuth2ProviderAppSecret = async (
		appId: string,
		secretId: string,
	): Promise<void> => {
		await this.axios.delete(
			`/api/v2/oauth2-provider/apps/${appId}/secrets/${secretId}`,
		);
	};

	revokeOAuth2ProviderApp = async (appId: string): Promise<void> => {
		await this.axios.delete(`/oauth2/tokens?client_id=${appId}`);
	};

	getAuditLogs = async (
		options: TypesGen.AuditLogsRequest,
	): Promise<TypesGen.AuditLogResponse> => {
		const url = getURLWithSearchParams("/api/v2/audit", options);
		const response = await this.axios.get(url);
		return response.data;
	};

	getTemplateDAUs = async (
		templateId: string,
	): Promise<TypesGen.DAUsResponse> => {
		const response = await this.axios.get(
			`/api/v2/templates/${templateId}/daus`,
		);

		return response.data;
	};

	getDeploymentDAUs = async (
		// Default to user's local timezone.
		// As /api/v2/insights/daus only accepts whole-number values for tz_offset
		// we truncate the tz offset down to the closest hour.
		offset = Math.trunc(new Date().getTimezoneOffset() / 60),
	): Promise<TypesGen.DAUsResponse> => {
		const response = await this.axios.get(
			`/api/v2/insights/daus?tz_offset=${offset}`,
		);

		return response.data;
	};

	getTemplateACLAvailable = async (
		templateId: string,
		options: TypesGen.UsersRequest,
	): Promise<TypesGen.ACLAvailable> => {
		const url = getURLWithSearchParams(
			`/api/v2/templates/${templateId}/acl/available`,
			options,
		).toString();

		const response = await this.axios.get(url);
		return response.data;
	};

	getTemplateACL = async (
		templateId: string,
	): Promise<TypesGen.TemplateACL> => {
		const response = await this.axios.get(
			`/api/v2/templates/${templateId}/acl`,
		);

		return response.data;
	};

	updateTemplateACL = async (
		templateId: string,
		data: TypesGen.UpdateTemplateACL,
	): Promise<{ message: string }> => {
		const response = await this.axios.patch(
			`/api/v2/templates/${templateId}/acl`,
			data,
		);

		return response.data;
	};

	getApplicationsHost = async (): Promise<TypesGen.AppHostResponse> => {
		const response = await this.axios.get("/api/v2/applications/host");
		return response.data;
	};

	getGroups = async (
		options: { userId?: string } = {},
	): Promise<TypesGen.Group[]> => {
		const params: Record<string, string> = {};
		if (options.userId !== undefined) {
			params.has_member = options.userId;
		}

		const response = await this.axios.get("/api/v2/groups", { params });
		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	getGroupsByOrganization = async (
		organization: string,
	): Promise<TypesGen.Group[]> => {
		const response = await this.axios.get(
			`/api/v2/organizations/${organization}/groups`,
		);
		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	createGroup = async (
		organization: string,
		data: TypesGen.CreateGroupRequest,
	): Promise<TypesGen.Group> => {
		const response = await this.axios.post(
			`/api/v2/organizations/${organization}/groups`,
			data,
		);
		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	getGroup = async (
		organization: string,
		groupName: string,
	): Promise<TypesGen.Group> => {
		const response = await this.axios.get(
			`/api/v2/organizations/${organization}/groups/${groupName}`,
		);
		return response.data;
	};

	patchGroup = async (
		groupId: string,
		data: TypesGen.PatchGroupRequest,
	): Promise<TypesGen.Group> => {
		const response = await this.axios.patch(`/api/v2/groups/${groupId}`, data);
		return response.data;
	};

	addMember = async (groupId: string, userId: string) => {
		return this.patchGroup(groupId, {
			name: "",
			add_users: [userId],
			remove_users: [],
			display_name: null,
			avatar_url: null,
			quota_allowance: null,
		});
	};

	removeMember = async (groupId: string, userId: string) => {
		return this.patchGroup(groupId, {
			name: "",
			add_users: [],
			remove_users: [userId],
			display_name: null,
			avatar_url: null,
			quota_allowance: null,
		});
	};

	deleteGroup = async (groupId: string): Promise<void> => {
		await this.axios.delete(`/api/v2/groups/${groupId}`);
	};

	getWorkspaceQuota = async (
		organizationName: string,
		username: string,
	): Promise<TypesGen.WorkspaceQuota> => {
		const response = await this.axios.get(
			`/api/v2/organizations/${encodeURIComponent(organizationName)}/members/${encodeURIComponent(username)}/workspace-quota`,
		);

		return response.data;
	};

	getAgentListeningPorts = async (
		agentID: string,
	): Promise<TypesGen.WorkspaceAgentListeningPortsResponse> => {
		const response = await this.axios.get(
			`/api/v2/workspaceagents/${agentID}/listening-ports`,
		);
		return response.data;
	};

	getWorkspaceAgentSharedPorts = async (
		workspaceID: string,
	): Promise<TypesGen.WorkspaceAgentPortShares> => {
		const response = await this.axios.get(
			`/api/v2/workspaces/${workspaceID}/port-share`,
		);
		return response.data;
	};

	upsertWorkspaceAgentSharedPort = async (
		workspaceID: string,
		req: TypesGen.UpsertWorkspaceAgentPortShareRequest,
	): Promise<TypesGen.WorkspaceAgentPortShares> => {
		const response = await this.axios.post(
			`/api/v2/workspaces/${workspaceID}/port-share`,
			req,
		);
		return response.data;
	};

	deleteWorkspaceAgentSharedPort = async (
		workspaceID: string,
		req: TypesGen.DeleteWorkspaceAgentPortShareRequest,
	): Promise<TypesGen.WorkspaceAgentPortShares> => {
		const response = await this.axios.delete(
			`/api/v2/workspaces/${workspaceID}/port-share`,
			{ data: req },
		);

		return response.data;
	};

	// getDeploymentSSHConfig is used by the VSCode-Extension.
	getDeploymentSSHConfig = async (): Promise<TypesGen.SSHConfigResponse> => {
		const response = await this.axios.get("/api/v2/deployment/ssh");
		return response.data;
	};

	getDeploymentConfig = async (): Promise<DeploymentConfig> => {
		const response = await this.axios.get("/api/v2/deployment/config");
		return response.data;
	};

	getDeploymentStats = async (): Promise<TypesGen.DeploymentStats> => {
		const response = await this.axios.get("/api/v2/deployment/stats");
		return response.data;
	};

	getReplicas = async (): Promise<TypesGen.Replica[]> => {
		const response = await this.axios.get("/api/v2/replicas");
		return response.data;
	};

	getFile = async (fileId: string): Promise<ArrayBuffer> => {
		const response = await this.axios.get<ArrayBuffer>(
			`/api/v2/files/${fileId}`,
			{ responseType: "arraybuffer" },
		);

		return response.data;
	};

	getWorkspaceProxyRegions = async (): Promise<
		TypesGen.RegionsResponse<TypesGen.Region>
	> => {
		const response =
			await this.axios.get<TypesGen.RegionsResponse<TypesGen.Region>>(
				"/api/v2/regions",
			);

		return response.data;
	};

	getWorkspaceProxies = async (): Promise<
		TypesGen.RegionsResponse<TypesGen.WorkspaceProxy>
	> => {
		const response = await this.axios.get<
			TypesGen.RegionsResponse<TypesGen.WorkspaceProxy>
		>("/api/v2/workspaceproxies");

		return response.data;
	};

	createWorkspaceProxy = async (
		b: TypesGen.CreateWorkspaceProxyRequest,
	): Promise<TypesGen.UpdateWorkspaceProxyResponse> => {
		const response = await this.axios.post("/api/v2/workspaceproxies", b);
		return response.data;
	};

	getAppearance = async (): Promise<TypesGen.AppearanceConfig> => {
		try {
			const response = await this.axios.get("/api/v2/appearance");
			return response.data || {};
		} catch (ex) {
			if (isAxiosError(ex) && ex.response?.status === 404) {
				return {
					application_name: "",
					docs_url: "",
					logo_url: "",
					announcement_banners: [],
					service_banner: {
						enabled: false,
					},
				};
			}

			throw ex;
		}
	};

	updateAppearance = async (
		b: TypesGen.AppearanceConfig,
	): Promise<TypesGen.AppearanceConfig> => {
		const response = await this.axios.put("/api/v2/appearance", b);
		return response.data;
	};

	/**
	 * @param organization Can be the organization's ID or name
	 */
	getTemplateExamples = async (): Promise<TypesGen.TemplateExample[]> => {
		const response = await this.axios.get("/api/v2/templates/examples");

		return response.data;
	};

	uploadFile = async (file: File): Promise<TypesGen.UploadResponse> => {
		const response = await this.axios.post("/api/v2/files", file, {
			headers: { "Content-Type": file.type },
		});

		return response.data;
	};

	getTemplateVersionLogs = async (
		versionId: string,
	): Promise<TypesGen.ProvisionerJobLog[]> => {
		const response = await this.axios.get<TypesGen.ProvisionerJobLog[]>(
			`/api/v2/templateversions/${versionId}/logs`,
		);
		return response.data;
	};

	updateWorkspaceVersion = async (
		workspace: TypesGen.Workspace,
	): Promise<TypesGen.WorkspaceBuild> => {
		const template = await this.getTemplate(workspace.template_id);
		return this.startWorkspace(workspace.id, template.active_version_id);
	};

	getWorkspaceBuildParameters = async (
		workspaceBuildId: TypesGen.WorkspaceBuild["id"],
	): Promise<TypesGen.WorkspaceBuildParameter[]> => {
		const response = await this.axios.get<TypesGen.WorkspaceBuildParameter[]>(
			`/api/v2/workspacebuilds/${workspaceBuildId}/parameters`,
		);

		return response.data;
	};

	getLicenses = async (): Promise<GetLicensesResponse[]> => {
		const response = await this.axios.get("/api/v2/licenses");
		return response.data;
	};

	createLicense = async (
		data: TypesGen.AddLicenseRequest,
	): Promise<TypesGen.AddLicenseRequest> => {
		const response = await this.axios.post("/api/v2/licenses", data);
		return response.data;
	};

	removeLicense = async (licenseId: number): Promise<void> => {
		await this.axios.delete(`/api/v2/licenses/${licenseId}`);
	};

	/** Steps to change the workspace version
	 * - Get the latest template to access the latest active version
	 * - Get the current build parameters
	 * - Get the template parameters
	 * - Update the build parameters and check if there are missed parameters for
	 *   the new version
	 *   - If there are missing parameters raise an error
	 * - Create a build with the version and updated build parameters
	 */
	changeWorkspaceVersion = async (
		workspace: TypesGen.Workspace,
		templateVersionId: string,
		newBuildParameters: TypesGen.WorkspaceBuildParameter[] = [],
	): Promise<TypesGen.WorkspaceBuild> => {
		const [currentBuildParameters, templateParameters] = await Promise.all([
			this.getWorkspaceBuildParameters(workspace.latest_build.id),
			this.getTemplateVersionRichParameters(templateVersionId),
		]);

		const missingParameters = getMissingParameters(
			currentBuildParameters,
			newBuildParameters,
			templateParameters,
		);

		if (missingParameters.length > 0) {
			throw new MissingBuildParameters(missingParameters, templateVersionId);
		}

		return this.postWorkspaceBuild(workspace.id, {
			transition: "start",
			template_version_id: templateVersionId,
			rich_parameter_values: newBuildParameters,
		});
	};

	/** Steps to update the workspace
	 * - Get the latest template to access the latest active version
	 * - Get the current build parameters
	 * - Get the template parameters
	 * - Update the build parameters and check if there are missed parameters for
	 *   the newest version
	 *   - If there are missing parameters raise an error
	 * - Create a build with the latest version and updated build parameters
	 */
	updateWorkspace = async (
		workspace: TypesGen.Workspace,
		newBuildParameters: TypesGen.WorkspaceBuildParameter[] = [],
	): Promise<TypesGen.WorkspaceBuild> => {
		const [template, oldBuildParameters] = await Promise.all([
			this.getTemplate(workspace.template_id),
			this.getWorkspaceBuildParameters(workspace.latest_build.id),
		]);

		const activeVersionId = template.active_version_id;
		const templateParameters =
			await this.getTemplateVersionRichParameters(activeVersionId);

		const missingParameters = getMissingParameters(
			oldBuildParameters,
			newBuildParameters,
			templateParameters,
		);

		if (missingParameters.length > 0) {
			throw new MissingBuildParameters(missingParameters, activeVersionId);
		}

		return this.postWorkspaceBuild(workspace.id, {
			transition: "start",
			template_version_id: activeVersionId,
			rich_parameter_values: newBuildParameters,
		});
	};

	getWorkspaceResolveAutostart = async (
		workspaceId: string,
	): Promise<TypesGen.ResolveAutostartResponse> => {
		const response = await this.axios.get(
			`/api/v2/workspaces/${workspaceId}/resolve-autostart`,
		);
		return response.data;
	};

	issueReconnectingPTYSignedToken = async (
		params: TypesGen.IssueReconnectingPTYSignedTokenRequest,
	): Promise<TypesGen.IssueReconnectingPTYSignedTokenResponse> => {
		const response = await this.axios.post(
			"/api/v2/applications/reconnecting-pty-signed-token",
			params,
		);

		return response.data;
	};

	getWorkspaceParameters = async (workspace: TypesGen.Workspace) => {
		const latestBuild = workspace.latest_build;
		const [templateVersionRichParameters, buildParameters] = await Promise.all([
			this.getTemplateVersionRichParameters(latestBuild.template_version_id),
			this.getWorkspaceBuildParameters(latestBuild.id),
		]);

		return {
			templateVersionRichParameters,
			buildParameters,
		};
	};

	getInsightsUserLatency = async (
		filters: InsightsParams,
	): Promise<TypesGen.UserLatencyInsightsResponse> => {
		const params = new URLSearchParams(filters);
		const response = await this.axios.get(
			`/api/v2/insights/user-latency?${params}`,
		);

		return response.data;
	};

	getInsightsUserActivity = async (
		filters: InsightsParams,
	): Promise<TypesGen.UserActivityInsightsResponse> => {
		const params = new URLSearchParams(filters);
		const response = await this.axios.get(
			`/api/v2/insights/user-activity?${params}`,
		);

		return response.data;
	};

	getInsightsUserStatusCounts = async (
		offset = Math.trunc(new Date().getTimezoneOffset() / 60),
	): Promise<TypesGen.GetUserStatusCountsResponse> => {
		const searchParams = new URLSearchParams({
			tz_offset: offset.toString(),
		});
		const response = await this.axios.get(
			`/api/v2/insights/user-status-counts?${searchParams}`,
		);

		return response.data;
	};

	getInsightsTemplate = async (
		params: InsightsTemplateParams,
	): Promise<TypesGen.TemplateInsightsResponse> => {
		const searchParams = new URLSearchParams(params);
		const response = await this.axios.get(
			`/api/v2/insights/templates?${searchParams}`,
		);

		return response.data;
	};

	getHealth = async (force = false) => {
		const params = new URLSearchParams({ force: force.toString() });
		const response = await this.axios.get<TypesGen.HealthcheckReport>(
			`/api/v2/debug/health?${params}`,
		);
		return response.data;
	};

	getHealthSettings = async (): Promise<TypesGen.HealthSettings> => {
		const res = await this.axios.get<TypesGen.HealthSettings>(
			"/api/v2/debug/health/settings",
		);

		return res.data;
	};

	updateHealthSettings = async (data: TypesGen.UpdateHealthSettings) => {
		const response = await this.axios.put<TypesGen.HealthSettings>(
			"/api/v2/debug/health/settings",
			data,
		);

		return response.data;
	};

	putFavoriteWorkspace = async (workspaceID: string) => {
		await this.axios.put(`/api/v2/workspaces/${workspaceID}/favorite`);
	};

	deleteFavoriteWorkspace = async (workspaceID: string) => {
		await this.axios.delete(`/api/v2/workspaces/${workspaceID}/favorite`);
	};

	getJFrogXRayScan = async (options: GetJFrogXRayScanParams) => {
		const searchParams = new URLSearchParams({
			workspace_id: options.workspaceId,
			agent_id: options.agentId,
		});

		try {
			const res = await this.axios.get<TypesGen.JFrogXrayScan>(
				`/api/v2/integrations/jfrog/xray-scan?${searchParams}`,
			);

			return res.data;
		} catch (error) {
			if (isAxiosError(error) && error.response?.status === 404) {
				// react-query library does not allow undefined to be returned as a
				// query result
				return null;
			}

			throw error;
		}
	};

	postWorkspaceUsage = async (
		workspaceID: string,
		options: PostWorkspaceUsageRequest,
	) => {
		const response = await this.axios.post(
			`/api/v2/workspaces/${workspaceID}/usage`,
			options,
		);

		return response.data;
	};

	getUserNotificationPreferences = async (userId: string) => {
		const res = await this.axios.get<TypesGen.NotificationPreference[] | null>(
			`/api/v2/users/${userId}/notifications/preferences`,
		);
		return res.data ?? [];
	};

	putUserNotificationPreferences = async (
		userId: string,
		req: TypesGen.UpdateUserNotificationPreferences,
	) => {
		const res = await this.axios.put<TypesGen.NotificationPreference[]>(
			`/api/v2/users/${userId}/notifications/preferences`,
			req,
		);
		return res.data;
	};

	getSystemNotificationTemplates = async () => {
		const res = await this.axios.get<TypesGen.NotificationTemplate[]>(
			"/api/v2/notifications/templates/system",
		);
		return res.data;
	};

	getNotificationDispatchMethods = async () => {
		const res = await this.axios.get<TypesGen.NotificationMethodsResponse>(
			"/api/v2/notifications/dispatch-methods",
		);
		return res.data;
	};

	updateNotificationTemplateMethod = async (
		templateId: string,
		req: TypesGen.UpdateNotificationTemplateMethod,
	) => {
		const res = await this.axios.put<void>(
			`/api/v2/notifications/templates/${templateId}/method`,
			req,
		);
		return res.data;
	};

	postTestNotification = async () => {
		await this.axios.post<void>("/api/v2/notifications/test");
	};

	requestOneTimePassword = async (
		req: TypesGen.RequestOneTimePasscodeRequest,
	) => {
		await this.axios.post<void>("/api/v2/users/otp/request", req);
	};

	changePasswordWithOTP = async (
		req: TypesGen.ChangePasswordWithOneTimePasscodeRequest,
	) => {
		await this.axios.post<void>("/api/v2/users/otp/change-password", req);
	};

	workspaceBuildTimings = async (workspaceBuildId: string) => {
		const res = await this.axios.get<TypesGen.WorkspaceBuildTimings>(
			`/api/v2/workspacebuilds/${workspaceBuildId}/timings`,
		);
		return res.data;
	};

	getProvisionerJobs = async (orgId: string) => {
		const res = await this.axios.get<TypesGen.ProvisionerJob[]>(
			`/api/v2/organizations/${orgId}/provisionerjobs`,
		);
		return res.data;
	};

	cancelProvisionerJob = async (job: TypesGen.ProvisionerJob) => {
		switch (job.type) {
			case "workspace_build":
				if (!job.input.workspace_build_id) {
					throw new Error("Workspace build ID is required to cancel this job");
				}
				return this.cancelWorkspaceBuild(job.input.workspace_build_id);

			case "template_version_import":
				if (!job.input.template_version_id) {
					throw new Error("Template version ID is required to cancel this job");
				}
				return this.cancelTemplateVersionBuild(job.input.template_version_id);

			case "template_version_dry_run":
				if (!job.input.template_version_id) {
					throw new Error("Template version ID is required to cancel this job");
				}
				return this.cancelTemplateVersionDryRun(
					job.input.template_version_id,
					job.id,
				);
		}
	};

	getAgentContainers = async (agentId: string, labels?: string[]) => {
		const params = new URLSearchParams(
			labels?.map((label) => ["label", label]),
		);

		const res =
			await this.axios.get<TypesGen.WorkspaceAgentListContainersResponse>(
				`/api/v2/workspaceagents/${agentId}/containers?${params.toString()}`,
			);
		return res.data;
	};
}

// This is a hard coded CSRF token/cookie pair for local development. In prod,
// the GoLang webserver generates a random cookie with a new token for each
// document request. For local development, we don't use the Go webserver for
// static files, so this is the 'hack' to make local development work with
// remote apis. The CSRF cookie for this token is "JXm9hOUdZctWt0ZZGAy9xiS/gxMKYOThdxjjMnMUyn4="
const csrfToken =
	"KNKvagCBEHZK7ihe2t7fj6VeJ0UyTDco1yVUJE8N06oNqxLu5Zx1vRxZbgfC0mJJgeGkVjgs08mgPbcWPBkZ1A==";

// Always attach CSRF token to all requests. In puppeteer the document is
// undefined. In those cases, just do nothing.
const tokenMetadataElement =
	typeof document !== "undefined"
		? document.head.querySelector('meta[property="csrf-token"]')
		: null;

function getConfiguredAxiosInstance(): AxiosInstance {
	const instance = globalAxios.create();

	// Adds 304 for the default axios validateStatus function
	// https://github.com/axios/axios#handling-errors Check status here
	// https://httpstatusdogs.com/
	instance.defaults.validateStatus = (status) => {
		return (status >= 200 && status < 300) || status === 304;
	};

	const metadataIsAvailable =
		tokenMetadataElement !== null &&
		tokenMetadataElement.getAttribute("content") !== null;

	if (metadataIsAvailable) {
		if (process.env.NODE_ENV === "development") {
			// Development mode uses a hard-coded CSRF token
			instance.defaults.headers.common["X-CSRF-TOKEN"] = csrfToken;
			instance.defaults.headers.common["X-CSRF-TOKEN"] = csrfToken;
			tokenMetadataElement.setAttribute("content", csrfToken);
		} else {
			instance.defaults.headers.common["X-CSRF-TOKEN"] =
				tokenMetadataElement.getAttribute("content") ?? "";
		}
	} else {
		// Do not write error logs if we are in a FE unit test.
		if (process.env.JEST_WORKER_ID === undefined) {
			console.error("CSRF token not found");
		}
	}

	return instance;
}

// Other non-API methods defined here to make it a little easier to find them.
interface ClientApi extends ApiMethods {
	getCsrfToken: () => string;
	setSessionToken: (token: string) => void;
	setHost: (host: string | undefined) => void;
	getAxiosInstance: () => AxiosInstance;
}

export class Api extends ApiMethods implements ClientApi {
	constructor() {
		const scopedAxiosInstance = getConfiguredAxiosInstance();
		super(scopedAxiosInstance);
	}

	// As with ApiMethods, all public methods should be defined with arrow
	// function syntax to ensure they can be passed around the React UI without
	// losing/detaching their `this` context!

	getCsrfToken = (): string => {
		return csrfToken;
	};

	setSessionToken = (token: string): void => {
		this.axios.defaults.headers.common["Coder-Session-Token"] = token;
	};

	setHost = (host: string | undefined): void => {
		this.axios.defaults.baseURL = host;
	};

	getAxiosInstance = (): AxiosInstance => {
		return this.axios;
	};
}

export const API = new Api();
