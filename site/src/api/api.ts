import axios from "axios";
import dayjs from "dayjs";
import * as Types from "./types";
import { DeploymentConfig } from "./types";
import * as TypesGen from "./typesGenerated";
import { delay } from "utils/delay";
import userAgentParser from "ua-parser-js";

// Adds 304 for the default axios validateStatus function
// https://github.com/axios/axios#handling-errors Check status here
// https://httpstatusdogs.com/
axios.defaults.validateStatus = (status) => {
  return (status >= 200 && status < 300) || status === 304;
};

export const hardCodedCSRFCookie = (): string => {
  // This is a hard coded CSRF token/cookie pair for local development. In prod,
  // the GoLang webserver generates a random cookie with a new token for each
  // document request. For local development, we don't use the Go webserver for
  // static files, so this is the 'hack' to make local development work with
  // remote apis. The CSRF cookie for this token is
  // "JXm9hOUdZctWt0ZZGAy9xiS/gxMKYOThdxjjMnMUyn4="
  const csrfToken =
    "KNKvagCBEHZK7ihe2t7fj6VeJ0UyTDco1yVUJE8N06oNqxLu5Zx1vRxZbgfC0mJJgeGkVjgs08mgPbcWPBkZ1A==";
  axios.defaults.headers.common["X-CSRF-TOKEN"] = csrfToken;
  return csrfToken;
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

// Always attach CSRF token to all requests. In puppeteer the document is
// undefined. In those cases, just do nothing.
const token =
  typeof document !== "undefined"
    ? document.head.querySelector('meta[property="csrf-token"]')
    : null;

if (token !== null && token.getAttribute("content") !== null) {
  if (process.env.NODE_ENV === "development") {
    // Development mode uses a hard-coded CSRF token
    axios.defaults.headers.common["X-CSRF-TOKEN"] = hardCodedCSRFCookie();
    token.setAttribute("content", hardCodedCSRFCookie());
  } else {
    axios.defaults.headers.common["X-CSRF-TOKEN"] =
      token.getAttribute("content") ?? "";
  }
} else {
  // Do not write error logs if we are in a FE unit test.
  if (process.env.JEST_WORKER_ID === undefined) {
    console.error("CSRF token not found");
  }
}

const CONTENT_TYPE_JSON = {
  "Content-Type": "application/json",
};

export const provisioners: TypesGen.ProvisionerDaemon[] = [
  {
    id: "terraform",
    name: "Terraform",
    created_at: "",
    provisioners: [],
    tags: {},
  },
  {
    id: "cdr-basic",
    name: "Basic",
    created_at: "",
    provisioners: [],
    tags: {},
  },
];

export const login = async (
  email: string,
  password: string,
): Promise<TypesGen.LoginWithPasswordResponse> => {
  const payload = JSON.stringify({
    email,
    password,
  });

  const response = await axios.post<TypesGen.LoginWithPasswordResponse>(
    "/api/v2/users/login",
    payload,
    {
      headers: { ...CONTENT_TYPE_JSON },
    },
  );

  return response.data;
};

export const convertToOAUTH = async (request: TypesGen.ConvertLoginRequest) => {
  const response = await axios.post<TypesGen.OAuthConversionResponse>(
    "/api/v2/users/me/convert-login",
    request,
  );
  return response.data;
};

export const logout = async (): Promise<void> => {
  await axios.post("/api/v2/users/logout");
};

export const getAuthenticatedUser = async (): Promise<
  TypesGen.User | undefined
> => {
  try {
    const response = await axios.get<TypesGen.User>("/api/v2/users/me");
    return response.data;
  } catch (error) {
    if (axios.isAxiosError(error) && error.response?.status === 401) {
      return undefined;
    }

    throw error;
  }
};

export const getAuthMethods = async (): Promise<TypesGen.AuthMethods> => {
  const response = await axios.get<TypesGen.AuthMethods>(
    "/api/v2/users/authmethods",
  );
  return response.data;
};

export const getUserLoginType = async (): Promise<TypesGen.UserLoginType> => {
  const response = await axios.get<TypesGen.UserLoginType>(
    "/api/v2/users/me/login-type",
  );
  return response.data;
};

export const checkAuthorization = async (
  params: TypesGen.AuthorizationRequest,
): Promise<TypesGen.AuthorizationResponse> => {
  const response = await axios.post<TypesGen.AuthorizationResponse>(
    `/api/v2/authcheck`,
    params,
  );
  return response.data;
};

export const getApiKey = async (): Promise<TypesGen.GenerateAPIKeyResponse> => {
  const response = await axios.post<TypesGen.GenerateAPIKeyResponse>(
    "/api/v2/users/me/keys",
  );
  return response.data;
};

export const getTokens = async (
  params: TypesGen.TokensFilter,
): Promise<TypesGen.APIKeyWithOwner[]> => {
  const response = await axios.get<TypesGen.APIKeyWithOwner[]>(
    `/api/v2/users/me/keys/tokens`,
    {
      params,
    },
  );
  return response.data;
};

export const deleteToken = async (keyId: string): Promise<void> => {
  await axios.delete("/api/v2/users/me/keys/" + keyId);
};

export const createToken = async (
  params: TypesGen.CreateTokenRequest,
): Promise<TypesGen.GenerateAPIKeyResponse> => {
  const response = await axios.post(`/api/v2/users/me/keys/tokens`, params);
  return response.data;
};

export const getTokenConfig = async (): Promise<TypesGen.TokenConfig> => {
  const response = await axios.get("/api/v2/users/me/keys/tokens/tokenconfig");
  return response.data;
};

export const getUsers = async (
  options: TypesGen.UsersRequest,
): Promise<TypesGen.GetUsersResponse> => {
  const url = getURLWithSearchParams("/api/v2/users", options);
  const response = await axios.get<TypesGen.GetUsersResponse>(url.toString());
  return response.data;
};

export const getOrganization = async (
  organizationId: string,
): Promise<TypesGen.Organization> => {
  const response = await axios.get<TypesGen.Organization>(
    `/api/v2/organizations/${organizationId}`,
  );
  return response.data;
};

export const getOrganizations = async (): Promise<TypesGen.Organization[]> => {
  const response = await axios.get<TypesGen.Organization[]>(
    "/api/v2/users/me/organizations",
  );
  return response.data;
};

export const getTemplate = async (
  templateId: string,
): Promise<TypesGen.Template> => {
  const response = await axios.get<TypesGen.Template>(
    `/api/v2/templates/${templateId}`,
  );
  return response.data;
};

export const getTemplates = async (
  organizationId: string,
): Promise<TypesGen.Template[]> => {
  const response = await axios.get<TypesGen.Template[]>(
    `/api/v2/organizations/${organizationId}/templates`,
  );
  return response.data;
};

export const getTemplateByName = async (
  organizationId: string,
  name: string,
): Promise<TypesGen.Template> => {
  const response = await axios.get<TypesGen.Template>(
    `/api/v2/organizations/${organizationId}/templates/${name}`,
  );
  return response.data;
};

export const getTemplateVersion = async (
  versionId: string,
): Promise<TypesGen.TemplateVersion> => {
  const response = await axios.get<TypesGen.TemplateVersion>(
    `/api/v2/templateversions/${versionId}`,
  );
  return response.data;
};

export const getTemplateVersionResources = async (
  versionId: string,
): Promise<TypesGen.WorkspaceResource[]> => {
  const response = await axios.get<TypesGen.WorkspaceResource[]>(
    `/api/v2/templateversions/${versionId}/resources`,
  );
  return response.data;
};

export const getTemplateVersionVariables = async (
  versionId: string,
): Promise<TypesGen.TemplateVersionVariable[]> => {
  const response = await axios.get<TypesGen.TemplateVersionVariable[]>(
    `/api/v2/templateversions/${versionId}/variables`,
  );
  return response.data;
};

export const getTemplateVersions = async (
  templateId: string,
): Promise<TypesGen.TemplateVersion[]> => {
  const response = await axios.get<TypesGen.TemplateVersion[]>(
    `/api/v2/templates/${templateId}/versions`,
  );
  return response.data;
};

export const getTemplateVersionByName = async (
  organizationId: string,
  templateName: string,
  versionName: string,
): Promise<TypesGen.TemplateVersion> => {
  const response = await axios.get<TypesGen.TemplateVersion>(
    `/api/v2/organizations/${organizationId}/templates/${templateName}/versions/${versionName}`,
  );
  return response.data;
};

export type GetPreviousTemplateVersionByNameResponse =
  | TypesGen.TemplateVersion
  | undefined;

export const getPreviousTemplateVersionByName = async (
  organizationId: string,
  templateName: string,
  versionName: string,
): Promise<GetPreviousTemplateVersionByNameResponse> => {
  try {
    const response = await axios.get<TypesGen.TemplateVersion>(
      `/api/v2/organizations/${organizationId}/templates/${templateName}/versions/${versionName}/previous`,
    );
    return response.data;
  } catch (error) {
    // When there is no previous version, like the first version of a template,
    // the API returns 404 so in this case we can safely return undefined
    if (
      axios.isAxiosError(error) &&
      error.response &&
      error.response.status === 404
    ) {
      return undefined;
    }

    throw error;
  }
};

export const createTemplateVersion = async (
  organizationId: string,
  data: TypesGen.CreateTemplateVersionRequest,
): Promise<TypesGen.TemplateVersion> => {
  const response = await axios.post<TypesGen.TemplateVersion>(
    `/api/v2/organizations/${organizationId}/templateversions`,
    data,
  );
  return response.data;
};

export const getTemplateVersionGitAuth = async (
  versionId: string,
): Promise<TypesGen.TemplateVersionGitAuth[]> => {
  const response = await axios.get(
    `/api/v2/templateversions/${versionId}/gitauth`,
  );
  return response.data;
};

export const getTemplateVersionRichParameters = async (
  versionId: string,
): Promise<TypesGen.TemplateVersionParameter[]> => {
  const response = await axios.get(
    `/api/v2/templateversions/${versionId}/rich-parameters`,
  );
  return response.data;
};

export const createTemplate = async (
  organizationId: string,
  data: TypesGen.CreateTemplateRequest,
): Promise<TypesGen.Template> => {
  const response = await axios.post(
    `/api/v2/organizations/${organizationId}/templates`,
    data,
  );
  return response.data;
};

export const updateActiveTemplateVersion = async (
  templateId: string,
  data: TypesGen.UpdateActiveTemplateVersion,
): Promise<Types.Message> => {
  const response = await axios.patch<Types.Message>(
    `/api/v2/templates/${templateId}/versions`,
    data,
  );
  return response.data;
};

export const patchTemplateVersion = async (
  templateVersionId: string,
  data: TypesGen.PatchTemplateVersionRequest,
) => {
  const response = await axios.patch<TypesGen.TemplateVersion>(
    `/api/v2/templateversions/${templateVersionId}`,
    data,
  );
  return response.data;
};

export const updateTemplateMeta = async (
  templateId: string,
  data: TypesGen.UpdateTemplateMeta,
): Promise<TypesGen.Template> => {
  const response = await axios.patch<TypesGen.Template>(
    `/api/v2/templates/${templateId}`,
    data,
  );
  return response.data;
};

export const deleteTemplate = async (
  templateId: string,
): Promise<TypesGen.Template> => {
  const response = await axios.delete<TypesGen.Template>(
    `/api/v2/templates/${templateId}`,
  );
  return response.data;
};

export const getWorkspace = async (
  workspaceId: string,
  params?: TypesGen.WorkspaceOptions,
): Promise<TypesGen.Workspace> => {
  const response = await axios.get<TypesGen.Workspace>(
    `/api/v2/workspaces/${workspaceId}`,
    {
      params,
    },
  );
  return response.data;
};

/**
 *
 * @param workspaceId
 * @returns An EventSource that emits workspace event objects (ServerSentEvent)
 */
export const watchWorkspace = (workspaceId: string): EventSource => {
  return new EventSource(
    `${location.protocol}//${location.host}/api/v2/workspaces/${workspaceId}/watch`,
    { withCredentials: true },
  );
};

interface SearchParamOptions extends TypesGen.Pagination {
  q?: string;
}

export const getURLWithSearchParams = (
  basePath: string,
  options?: SearchParamOptions,
): string => {
  if (options) {
    const searchParams = new URLSearchParams();
    const keys = Object.keys(options) as (keyof SearchParamOptions)[];
    keys.forEach((key) => {
      const value = options[key];
      if (value !== undefined && value !== "") {
        searchParams.append(key, value.toString());
      }
    });
    const searchString = searchParams.toString();
    return searchString ? `${basePath}?${searchString}` : basePath;
  } else {
    return basePath;
  }
};

export const getWorkspaces = async (
  options: TypesGen.WorkspacesRequest,
): Promise<TypesGen.WorkspacesResponse> => {
  const url = getURLWithSearchParams("/api/v2/workspaces", options);
  const response = await axios.get<TypesGen.WorkspacesResponse>(url);
  return response.data;
};

export const getWorkspaceByOwnerAndName = async (
  username = "me",
  workspaceName: string,
  params?: TypesGen.WorkspaceOptions,
): Promise<TypesGen.Workspace> => {
  const response = await axios.get<TypesGen.Workspace>(
    `/api/v2/users/${username}/workspace/${workspaceName}`,
    {
      params,
    },
  );
  return response.data;
};

export function waitForBuild(build: TypesGen.WorkspaceBuild) {
  return new Promise<TypesGen.ProvisionerJob | undefined>((res, reject) => {
    void (async () => {
      let latestJobInfo: TypesGen.ProvisionerJob | undefined = undefined;

      while (
        !["succeeded", "canceled"].some(
          (status) => latestJobInfo?.status.includes(status),
        )
      ) {
        const { job } = await getWorkspaceBuildByNumber(
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
}

export const postWorkspaceBuild = async (
  workspaceId: string,
  data: TypesGen.CreateWorkspaceBuildRequest,
): Promise<TypesGen.WorkspaceBuild> => {
  const response = await axios.post(
    `/api/v2/workspaces/${workspaceId}/builds`,
    data,
  );
  return response.data;
};

export const startWorkspace = (
  workspaceId: string,
  templateVersionId: string,
  logLevel?: TypesGen.CreateWorkspaceBuildRequest["log_level"],
  buildParameters?: TypesGen.WorkspaceBuildParameter[],
) =>
  postWorkspaceBuild(workspaceId, {
    transition: "start",
    template_version_id: templateVersionId,
    log_level: logLevel,
    rich_parameter_values: buildParameters,
  });
export const stopWorkspace = (
  workspaceId: string,
  logLevel?: TypesGen.CreateWorkspaceBuildRequest["log_level"],
) =>
  postWorkspaceBuild(workspaceId, {
    transition: "stop",
    log_level: logLevel,
  });

export const deleteWorkspace = (
  workspaceId: string,
  logLevel?: TypesGen.CreateWorkspaceBuildRequest["log_level"],
) =>
  postWorkspaceBuild(workspaceId, {
    transition: "delete",
    log_level: logLevel,
  });

export const cancelWorkspaceBuild = async (
  workspaceBuildId: TypesGen.WorkspaceBuild["id"],
): Promise<Types.Message> => {
  const response = await axios.patch(
    `/api/v2/workspacebuilds/${workspaceBuildId}/cancel`,
  );
  return response.data;
};

export const updateWorkspaceDormancy = async (
  workspaceId: string,
  dormant: boolean,
): Promise<TypesGen.Workspace> => {
  const data: TypesGen.UpdateWorkspaceDormancy = {
    dormant: dormant,
  };

  const response = await axios.put(
    `/api/v2/workspaces/${workspaceId}/dormant`,
    data,
  );
  return response.data;
};

export const restartWorkspace = async ({
  workspace,
  buildParameters,
}: {
  workspace: TypesGen.Workspace;
  buildParameters?: TypesGen.WorkspaceBuildParameter[];
}) => {
  const stopBuild = await stopWorkspace(workspace.id);
  const awaitedStopBuild = await waitForBuild(stopBuild);

  // If the restart is canceled halfway through, make sure we bail
  if (awaitedStopBuild?.status === "canceled") {
    return;
  }

  const startBuild = await startWorkspace(
    workspace.id,
    workspace.latest_build.template_version_id,
    undefined,
    buildParameters,
  );
  await waitForBuild(startBuild);
};

export const cancelTemplateVersionBuild = async (
  templateVersionId: TypesGen.TemplateVersion["id"],
): Promise<Types.Message> => {
  const response = await axios.patch(
    `/api/v2/templateversions/${templateVersionId}/cancel`,
  );
  return response.data;
};

export const createUser = async (
  user: TypesGen.CreateUserRequest,
): Promise<TypesGen.User> => {
  const response = await axios.post<TypesGen.User>("/api/v2/users", user);
  return response.data;
};

export const createWorkspace = async (
  organizationId: string,
  userId = "me",
  workspace: TypesGen.CreateWorkspaceRequest,
): Promise<TypesGen.Workspace> => {
  const response = await axios.post<TypesGen.Workspace>(
    `/api/v2/organizations/${organizationId}/members/${userId}/workspaces`,
    workspace,
  );
  return response.data;
};

export const patchWorkspace = async (
  workspaceId: string,
  data: TypesGen.UpdateWorkspaceRequest,
) => {
  await axios.patch(`/api/v2/workspaces/${workspaceId}`, data);
};

export const getBuildInfo = async (): Promise<TypesGen.BuildInfoResponse> => {
  const response = await axios.get("/api/v2/buildinfo");
  return response.data;
};

export const getUpdateCheck =
  async (): Promise<TypesGen.UpdateCheckResponse> => {
    const response = await axios.get("/api/v2/updatecheck");
    return response.data;
  };

export const putWorkspaceAutostart = async (
  workspaceID: string,
  autostart: TypesGen.UpdateWorkspaceAutostartRequest,
): Promise<void> => {
  const payload = JSON.stringify(autostart);
  await axios.put(`/api/v2/workspaces/${workspaceID}/autostart`, payload, {
    headers: { ...CONTENT_TYPE_JSON },
  });
};

export const putWorkspaceAutostop = async (
  workspaceID: string,
  ttl: TypesGen.UpdateWorkspaceTTLRequest,
): Promise<void> => {
  const payload = JSON.stringify(ttl);
  await axios.put(`/api/v2/workspaces/${workspaceID}/ttl`, payload, {
    headers: { ...CONTENT_TYPE_JSON },
  });
};

export const updateProfile = async (
  userId: string,
  data: TypesGen.UpdateUserProfileRequest,
): Promise<TypesGen.User> => {
  const response = await axios.put(`/api/v2/users/${userId}/profile`, data);
  return response.data;
};

export const activateUser = async (
  userId: TypesGen.User["id"],
): Promise<TypesGen.User> => {
  const response = await axios.put<TypesGen.User>(
    `/api/v2/users/${userId}/status/activate`,
  );
  return response.data;
};

export const suspendUser = async (
  userId: TypesGen.User["id"],
): Promise<TypesGen.User> => {
  const response = await axios.put<TypesGen.User>(
    `/api/v2/users/${userId}/status/suspend`,
  );
  return response.data;
};

export const deleteUser = async (
  userId: TypesGen.User["id"],
): Promise<undefined> => {
  return await axios.delete(`/api/v2/users/${userId}`);
};

// API definition:
// https://github.com/coder/coder/blob/db665e7261f3c24a272ccec48233a3e276878239/coderd/users.go#L33-L53
export const hasFirstUser = async (): Promise<boolean> => {
  try {
    // If it is success, it is true
    await axios.get("/api/v2/users/first");
    return true;
  } catch (error) {
    // If it returns a 404, it is false
    if (axios.isAxiosError(error) && error.response?.status === 404) {
      return false;
    }

    throw error;
  }
};

export const createFirstUser = async (
  req: TypesGen.CreateFirstUserRequest,
): Promise<TypesGen.CreateFirstUserResponse> => {
  const response = await axios.post(`/api/v2/users/first`, req);
  return response.data;
};

export const updateUserPassword = async (
  userId: TypesGen.User["id"],
  updatePassword: TypesGen.UpdateUserPasswordRequest,
): Promise<undefined> =>
  axios.put(`/api/v2/users/${userId}/password`, updatePassword);

export const getSiteRoles = async (): Promise<
  Array<TypesGen.AssignableRoles>
> => {
  const response = await axios.get<Array<TypesGen.AssignableRoles>>(
    `/api/v2/users/roles`,
  );
  return response.data;
};

export const updateUserRoles = async (
  roles: TypesGen.Role["name"][],
  userId: TypesGen.User["id"],
): Promise<TypesGen.User> => {
  const response = await axios.put<TypesGen.User>(
    `/api/v2/users/${userId}/roles`,
    { roles },
  );
  return response.data;
};

export const getUserSSHKey = async (
  userId = "me",
): Promise<TypesGen.GitSSHKey> => {
  const response = await axios.get<TypesGen.GitSSHKey>(
    `/api/v2/users/${userId}/gitsshkey`,
  );
  return response.data;
};

export const regenerateUserSSHKey = async (
  userId = "me",
): Promise<TypesGen.GitSSHKey> => {
  const response = await axios.put<TypesGen.GitSSHKey>(
    `/api/v2/users/${userId}/gitsshkey`,
  );
  return response.data;
};

export const getWorkspaceBuilds = async (
  workspaceId: string,
  since: Date,
): Promise<TypesGen.WorkspaceBuild[]> => {
  const response = await axios.get<TypesGen.WorkspaceBuild[]>(
    `/api/v2/workspaces/${workspaceId}/builds?since=${since.toISOString()}`,
  );
  return response.data;
};

export const getWorkspaceBuildByNumber = async (
  username = "me",
  workspaceName: string,
  buildNumber: number,
): Promise<TypesGen.WorkspaceBuild> => {
  const response = await axios.get<TypesGen.WorkspaceBuild>(
    `/api/v2/users/${username}/workspace/${workspaceName}/builds/${buildNumber}`,
  );
  return response.data;
};

export const getWorkspaceBuildLogs = async (
  buildname: string,
  before: Date,
): Promise<TypesGen.ProvisionerJobLog[]> => {
  const response = await axios.get<TypesGen.ProvisionerJobLog[]>(
    `/api/v2/workspacebuilds/${buildname}/logs?before=${before.getTime()}`,
  );
  return response.data;
};

export const getWorkspaceAgentLogs = async (
  agentID: string,
): Promise<TypesGen.WorkspaceAgentLog[]> => {
  const response = await axios.get<TypesGen.WorkspaceAgentLog[]>(
    `/api/v2/workspaceagents/${agentID}/logs`,
  );
  return response.data;
};

export const putWorkspaceExtension = async (
  workspaceId: string,
  newDeadline: dayjs.Dayjs,
): Promise<void> => {
  await axios.put(`/api/v2/workspaces/${workspaceId}/extend`, {
    deadline: newDeadline,
  });
};

export const refreshEntitlements = async (): Promise<void> => {
  await axios.post("/api/v2/licenses/refresh-entitlements");
};

export const getEntitlements = async (): Promise<TypesGen.Entitlements> => {
  try {
    const response = await axios.get("/api/v2/entitlements");
    return response.data;
  } catch (ex) {
    if (axios.isAxiosError(ex) && ex.response?.status === 404) {
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

export const getExperiments = async (): Promise<TypesGen.Experiment[]> => {
  try {
    const response = await axios.get("/api/v2/experiments");
    return response.data;
  } catch (error) {
    if (axios.isAxiosError(error) && error.response?.status === 404) {
      return [];
    }
    throw error;
  }
};

export const getGitAuthProvider = async (
  provider: string,
): Promise<TypesGen.GitAuth> => {
  const resp = await axios.get(`/api/v2/gitauth/${provider}`);
  return resp.data;
};

export const getGitAuthDevice = async (
  provider: string,
): Promise<TypesGen.GitAuthDevice> => {
  const resp = await axios.get(`/api/v2/gitauth/${provider}/device`);
  return resp.data;
};

export const exchangeGitAuthDevice = async (
  provider: string,
  req: TypesGen.GitAuthDeviceExchange,
): Promise<void> => {
  const resp = await axios.post(`/api/v2/gitauth/${provider}/device`, req);
  return resp.data;
};

export const getAuditLogs = async (
  options: TypesGen.AuditLogsRequest,
): Promise<TypesGen.AuditLogResponse> => {
  const url = getURLWithSearchParams("/api/v2/audit", options);
  const response = await axios.get(url);
  return response.data;
};

export const getTemplateDAUs = async (
  templateId: string,
): Promise<TypesGen.DAUsResponse> => {
  const response = await axios.get(`/api/v2/templates/${templateId}/daus`);
  return response.data;
};

export const getDeploymentDAUs = async (
  // Default to user's local timezone
  offset = new Date().getTimezoneOffset() / 60,
): Promise<TypesGen.DAUsResponse> => {
  const response = await axios.get(`/api/v2/insights/daus?tz_offset=${offset}`);
  return response.data;
};

export const getTemplateACLAvailable = async (
  templateId: string,
  options: TypesGen.UsersRequest,
): Promise<TypesGen.ACLAvailable> => {
  const url = getURLWithSearchParams(
    `/api/v2/templates/${templateId}/acl/available`,
    options,
  );
  const response = await axios.get(url.toString());
  return response.data;
};

export const getTemplateACL = async (
  templateId: string,
): Promise<TypesGen.TemplateACL> => {
  const response = await axios.get(`/api/v2/templates/${templateId}/acl`);
  return response.data;
};

export const updateTemplateACL = async (
  templateId: string,
  data: TypesGen.UpdateTemplateACL,
): Promise<TypesGen.TemplateACL> => {
  const response = await axios.patch(
    `/api/v2/templates/${templateId}/acl`,
    data,
  );
  return response.data;
};

export const getApplicationsHost =
  async (): Promise<TypesGen.AppHostResponse> => {
    const response = await axios.get(`/api/v2/applications/host`);
    return response.data;
  };

export const getGroups = async (
  organizationId: string,
): Promise<TypesGen.Group[]> => {
  const response = await axios.get(
    `/api/v2/organizations/${organizationId}/groups`,
  );
  return response.data;
};

export const createGroup = async (
  organizationId: string,
  data: TypesGen.CreateGroupRequest,
): Promise<TypesGen.Group> => {
  const response = await axios.post(
    `/api/v2/organizations/${organizationId}/groups`,
    data,
  );
  return response.data;
};

export const getGroup = async (groupId: string): Promise<TypesGen.Group> => {
  const response = await axios.get(`/api/v2/groups/${groupId}`);
  return response.data;
};

export const patchGroup = async (
  groupId: string,
  data: TypesGen.PatchGroupRequest,
): Promise<TypesGen.Group> => {
  const response = await axios.patch(`/api/v2/groups/${groupId}`, data);
  return response.data;
};

export const deleteGroup = async (groupId: string): Promise<void> => {
  await axios.delete(`/api/v2/groups/${groupId}`);
};

export const getWorkspaceQuota = async (
  userID: string,
): Promise<TypesGen.WorkspaceQuota> => {
  const response = await axios.get(`/api/v2/workspace-quota/${userID}`);
  return response.data;
};

export const getAgentListeningPorts = async (
  agentID: string,
): Promise<TypesGen.WorkspaceAgentListeningPortsResponse> => {
  const response = await axios.get(
    `/api/v2/workspaceagents/${agentID}/listening-ports`,
  );
  return response.data;
};

// getDeploymentSSHConfig is used by the VSCode-Extension.
export const getDeploymentSSHConfig =
  async (): Promise<TypesGen.SSHConfigResponse> => {
    const response = await axios.get(`/api/v2/deployment/ssh`);
    return response.data;
  };

export const getDeploymentValues = async (): Promise<DeploymentConfig> => {
  const response = await axios.get(`/api/v2/deployment/config`);
  return response.data;
};

export const getDeploymentStats =
  async (): Promise<TypesGen.DeploymentStats> => {
    const response = await axios.get(`/api/v2/deployment/stats`);
    return response.data;
  };

export const getReplicas = async (): Promise<TypesGen.Replica[]> => {
  const response = await axios.get(`/api/v2/replicas`);
  return response.data;
};

export const getFile = async (fileId: string): Promise<ArrayBuffer> => {
  const response = await axios.get<ArrayBuffer>(`/api/v2/files/${fileId}`, {
    responseType: "arraybuffer",
  });
  return response.data;
};

export const getWorkspaceProxyRegions = async (): Promise<
  TypesGen.RegionsResponse<TypesGen.Region>
> => {
  const response = await axios.get<TypesGen.RegionsResponse<TypesGen.Region>>(
    `/api/v2/regions`,
  );
  return response.data;
};

export const getWorkspaceProxies = async (): Promise<
  TypesGen.RegionsResponse<TypesGen.WorkspaceProxy>
> => {
  const response = await axios.get<
    TypesGen.RegionsResponse<TypesGen.WorkspaceProxy>
  >(`/api/v2/workspaceproxies`);
  return response.data;
};

export const getAppearance = async (): Promise<TypesGen.AppearanceConfig> => {
  try {
    const response = await axios.get(`/api/v2/appearance`);
    return response.data || {};
  } catch (ex) {
    if (axios.isAxiosError(ex) && ex.response?.status === 404) {
      return {
        logo_url: "",
        service_banner: {
          enabled: false,
        },
      };
    }
    throw ex;
  }
};

export const updateAppearance = async (
  b: TypesGen.AppearanceConfig,
): Promise<TypesGen.AppearanceConfig> => {
  const response = await axios.put(`/api/v2/appearance`, b);
  return response.data;
};

export const getTemplateExamples = async (
  organizationId: string,
): Promise<TypesGen.TemplateExample[]> => {
  const response = await axios.get(
    `/api/v2/organizations/${organizationId}/templates/examples`,
  );
  return response.data;
};

export const uploadTemplateFile = async (
  file: File,
): Promise<TypesGen.UploadResponse> => {
  const response = await axios.post("/api/v2/files", file, {
    headers: {
      "Content-Type": "application/x-tar",
    },
  });
  return response.data;
};

export const getTemplateVersionLogs = async (
  versionId: string,
): Promise<TypesGen.ProvisionerJobLog[]> => {
  const response = await axios.get<TypesGen.ProvisionerJobLog[]>(
    `/api/v2/templateversions/${versionId}/logs`,
  );
  return response.data;
};

export const updateWorkspaceVersion = async (
  workspace: TypesGen.Workspace,
): Promise<TypesGen.WorkspaceBuild> => {
  const template = await getTemplate(workspace.template_id);
  return startWorkspace(workspace.id, template.active_version_id);
};

export const getWorkspaceBuildParameters = async (
  workspaceBuildId: TypesGen.WorkspaceBuild["id"],
): Promise<TypesGen.WorkspaceBuildParameter[]> => {
  const response = await axios.get<TypesGen.WorkspaceBuildParameter[]>(
    `/api/v2/workspacebuilds/${workspaceBuildId}/parameters`,
  );
  return response.data;
};
type Claims = {
  license_expires: number;
  account_type?: string;
  account_id?: string;
  trial: boolean;
  all_features: boolean;
  version: number;
  features: Record<string, number>;
  require_telemetry?: boolean;
};

export type GetLicensesResponse = Omit<TypesGen.License, "claims"> & {
  claims: Claims;
  expires_at: string;
};

export const getLicenses = async (): Promise<GetLicensesResponse[]> => {
  const response = await axios.get(`/api/v2/licenses`);
  return response.data;
};

export const createLicense = async (
  data: TypesGen.AddLicenseRequest,
): Promise<TypesGen.AddLicenseRequest> => {
  const response = await axios.post(`/api/v2/licenses`, data);
  return response.data;
};

export const removeLicense = async (licenseId: number): Promise<void> => {
  await axios.delete(`/api/v2/licenses/${licenseId}`);
};

export class MissingBuildParameters extends Error {
  parameters: TypesGen.TemplateVersionParameter[] = [];

  constructor(parameters: TypesGen.TemplateVersionParameter[]) {
    super("Missing build parameters.");
    this.parameters = parameters;
  }
}

/** Steps to change the workspace version
 * - Get the latest template to access the latest active version
 * - Get the current build parameters
 * - Get the template parameters
 * - Update the build parameters and check if there are missed parameters for the new version
 *   - If there are missing parameters raise an error
 * - Create a build with the version and updated build parameters
 */
export const changeWorkspaceVersion = async (
  workspace: TypesGen.Workspace,
  templateVersionId: string,
  newBuildParameters: TypesGen.WorkspaceBuildParameter[] = [],
): Promise<TypesGen.WorkspaceBuild> => {
  const [currentBuildParameters, templateParameters] = await Promise.all([
    getWorkspaceBuildParameters(workspace.latest_build.id),
    getTemplateVersionRichParameters(templateVersionId),
  ]);

  const missingParameters = getMissingParameters(
    currentBuildParameters,
    newBuildParameters,
    templateParameters,
  );

  if (missingParameters.length > 0) {
    throw new MissingBuildParameters(missingParameters);
  }

  return postWorkspaceBuild(workspace.id, {
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
export const updateWorkspace = async (
  workspace: TypesGen.Workspace,
  newBuildParameters: TypesGen.WorkspaceBuildParameter[] = [],
): Promise<TypesGen.WorkspaceBuild> => {
  const [template, oldBuildParameters] = await Promise.all([
    getTemplate(workspace.template_id),
    getWorkspaceBuildParameters(workspace.latest_build.id),
  ]);
  const activeVersionId = template.active_version_id;
  const templateParameters = await getTemplateVersionRichParameters(
    activeVersionId,
  );
  const missingParameters = getMissingParameters(
    oldBuildParameters,
    newBuildParameters,
    templateParameters,
  );

  if (missingParameters.length > 0) {
    throw new MissingBuildParameters(missingParameters);
  }

  return postWorkspaceBuild(workspace.id, {
    transition: "start",
    template_version_id: activeVersionId,
    rich_parameter_values: newBuildParameters,
  });
};

const getMissingParameters = (
  oldBuildParameters: TypesGen.WorkspaceBuildParameter[],
  newBuildParameters: TypesGen.WorkspaceBuildParameter[],
  templateParameters: TypesGen.TemplateVersionParameter[],
) => {
  const missingParameters: TypesGen.TemplateVersionParameter[] = [];
  const requiredParameters: TypesGen.TemplateVersionParameter[] = [];

  templateParameters.forEach((p) => {
    // It is mutable and required. Mutable values can be changed after so we
    // don't need to ask them if they are not required.
    const isMutableAndRequired = p.mutable && p.required;
    // Is immutable, so we can check if it is its first time on the build
    const isImmutable = !p.mutable;

    if (isMutableAndRequired || isImmutable) {
      requiredParameters.push(p);
    }
  });

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
  templateParameters.forEach((templateParameter) => {
    if (templateParameter.options.length === 0) {
      return;
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
      return;
    }

    const matchingOption = templateParameter.options.find(
      (option) => option.value === buildParameter?.value,
    );
    if (!matchingOption) {
      missingParameters.push(templateParameter);
    }
  });
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

type WatchBuildLogsByTemplateVersionIdOptions = {
  after?: number;
  onMessage: (log: TypesGen.ProvisionerJobLog) => void;
  onDone: () => void;
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
    onDone();
  });
  return socket;
};

type WatchWorkspaceAgentLogsOptions = {
  after: number;
  onMessage: (logs: TypesGen.WorkspaceAgentLog[]) => void;
  onDone: () => void;
  onError: (error: Error) => void;
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
    onDone();
  });

  return socket;
};

type WatchBuildLogsByBuildIdOptions = {
  after?: number;
  onMessage: (log: TypesGen.ProvisionerJobLog) => void;
  onDone: () => void;
  onError: (error: Error) => void;
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
    onError(new Error("Connection for logs failed."));
    socket.close();
  });
  socket.addEventListener("close", () => {
    // When the socket closes, logs have finished streaming!
    onDone();
  });
  return socket;
};

export const issueReconnectingPTYSignedToken = async (
  params: TypesGen.IssueReconnectingPTYSignedTokenRequest,
): Promise<TypesGen.IssueReconnectingPTYSignedTokenResponse> => {
  const response = await axios.post(
    "/api/v2/applications/reconnecting-pty-signed-token",
    params,
  );
  return response.data;
};

export const getWorkspaceParameters = async (workspace: TypesGen.Workspace) => {
  const latestBuild = workspace.latest_build;
  const [templateVersionRichParameters, buildParameters] = await Promise.all([
    getTemplateVersionRichParameters(latestBuild.template_version_id),
    getWorkspaceBuildParameters(latestBuild.id),
  ]);
  return {
    templateVersionRichParameters,
    buildParameters,
  };
};

type InsightsFilter = {
  start_time: string;
  end_time: string;
  template_ids: string;
};

export const getInsightsUserLatency = async (
  filters: InsightsFilter,
): Promise<TypesGen.UserLatencyInsightsResponse> => {
  const params = new URLSearchParams(filters);
  const response = await axios.get(`/api/v2/insights/user-latency?${params}`);
  return response.data;
};

export const getInsightsTemplate = async (
  filters: InsightsFilter,
): Promise<TypesGen.TemplateInsightsResponse> => {
  const params = new URLSearchParams({
    ...filters,
    interval: "day",
  });
  const response = await axios.get(`/api/v2/insights/templates?${params}`);
  return response.data;
};

export const getHealth = () => {
  return axios.get<{
    healthy: boolean;
    time: string;
    coder_version: string;
    derp: { healthy: boolean };
    access_url: { healthy: boolean };
    websocket: { healthy: boolean };
    database: { healthy: boolean };
  }>("/api/v2/debug/health");
};
