import axios, { AxiosRequestHeaders } from "axios"
import dayjs from "dayjs"
import * as Types from "./types"
import { WorkspaceBuildTransition } from "./types"
import * as TypesGen from "./typesGenerated"

export const hardCodedCSRFCookie = (): string => {
  // This is a hard coded CSRF token/cookie pair for local development.
  // In prod, the GoLang webserver generates a random cookie with a new token for
  // each document request. For local development, we don't use the Go webserver for static files,
  // so this is the 'hack' to make local development work with remote apis.
  // The CSRF cookie for this token is "JXm9hOUdZctWt0ZZGAy9xiS/gxMKYOThdxjjMnMUyn4="
  const csrfToken =
    "KNKvagCBEHZK7ihe2t7fj6VeJ0UyTDco1yVUJE8N06oNqxLu5Zx1vRxZbgfC0mJJgeGkVjgs08mgPbcWPBkZ1A=="
  axios.defaults.headers.common["X-CSRF-TOKEN"] = csrfToken
  return csrfToken
}

// defaultEntitlements has a default set of disabled functionality.
export const defaultEntitlements = (): TypesGen.Entitlements => {
  const features: TypesGen.Entitlements["features"] = {}
  for (const feature in Types.FeatureNames) {
    features[feature] = {
      enabled: false,
      entitlement: "not_entitled",
    }
  }
  return {
    features: features,
    has_license: false,
    errors: [],
    warnings: [],
    experimental: false,
    trial: false,
  }
}

// Always attach CSRF token to all requests.
// In puppeteer the document is undefined. In those cases, just
// do nothing.
const token =
  typeof document !== "undefined"
    ? document.head.querySelector('meta[property="csrf-token"]')
    : null

if (token !== null && token.getAttribute("content") !== null) {
  if (process.env.NODE_ENV === "development") {
    // Development mode uses a hard-coded CSRF token
    axios.defaults.headers.common["X-CSRF-TOKEN"] = hardCodedCSRFCookie()
    token.setAttribute("content", hardCodedCSRFCookie())
  } else {
    axios.defaults.headers.common["X-CSRF-TOKEN"] =
      token.getAttribute("content") ?? ""
  }
} else {
  // Do not write error logs if we are in a FE unit test.
  if (process.env.JEST_WORKER_ID === undefined) {
    console.error("CSRF token not found")
  }
}

const CONTENT_TYPE_JSON: AxiosRequestHeaders = {
  "Content-Type": "application/json",
}

export const provisioners: TypesGen.ProvisionerDaemon[] = [
  {
    id: "terraform",
    name: "Terraform",
    created_at: "",
    provisioners: [],
  },
  {
    id: "cdr-basic",
    name: "Basic",
    created_at: "",
    provisioners: [],
  },
]

export const login = async (
  email: string,
  password: string,
): Promise<TypesGen.LoginWithPasswordResponse> => {
  const payload = JSON.stringify({
    email,
    password,
  })

  const response = await axios.post<TypesGen.LoginWithPasswordResponse>(
    "/api/v2/users/login",
    payload,
    {
      headers: { ...CONTENT_TYPE_JSON },
    },
  )

  return response.data
}

export const logout = async (): Promise<void> => {
  await axios.post("/api/v2/users/logout")
}

export const getUser = async (): Promise<TypesGen.User> => {
  const response = await axios.get<TypesGen.User>("/api/v2/users/me")
  return response.data
}

export const getAuthMethods = async (): Promise<TypesGen.AuthMethods> => {
  const response = await axios.get<TypesGen.AuthMethods>(
    "/api/v2/users/authmethods",
  )
  return response.data
}

export const checkAuthorization = async (
  params: TypesGen.AuthorizationRequest,
): Promise<TypesGen.AuthorizationResponse> => {
  const response = await axios.post<TypesGen.AuthorizationResponse>(
    `/api/v2/authcheck`,
    params,
  )
  return response.data
}

export const getApiKey = async (): Promise<TypesGen.GenerateAPIKeyResponse> => {
  const response = await axios.post<TypesGen.GenerateAPIKeyResponse>(
    "/api/v2/users/me/keys",
  )
  return response.data
}

export const getUsers = async (
  options: TypesGen.UsersRequest,
): Promise<TypesGen.GetUsersResponse> => {
  const url = getURLWithSearchParams("/api/v2/users", options)
  const response = await axios.get<TypesGen.GetUsersResponse>(url.toString())
  return response.data
}

export const getOrganization = async (
  organizationId: string,
): Promise<TypesGen.Organization> => {
  const response = await axios.get<TypesGen.Organization>(
    `/api/v2/organizations/${organizationId}`,
  )
  return response.data
}

export const getOrganizations = async (): Promise<TypesGen.Organization[]> => {
  const response = await axios.get<TypesGen.Organization[]>(
    "/api/v2/users/me/organizations",
  )
  return response.data
}

export const getTemplate = async (
  templateId: string,
): Promise<TypesGen.Template> => {
  const response = await axios.get<TypesGen.Template>(
    `/api/v2/templates/${templateId}`,
  )
  return response.data
}

export const getTemplates = async (
  organizationId: string,
): Promise<TypesGen.Template[]> => {
  const response = await axios.get<TypesGen.Template[]>(
    `/api/v2/organizations/${organizationId}/templates`,
  )
  return response.data
}

export const getTemplateByName = async (
  organizationId: string,
  name: string,
): Promise<TypesGen.Template> => {
  const response = await axios.get<TypesGen.Template>(
    `/api/v2/organizations/${organizationId}/templates/${name}`,
  )
  return response.data
}

export const getTemplateVersion = async (
  versionId: string,
): Promise<TypesGen.TemplateVersion> => {
  const response = await axios.get<TypesGen.TemplateVersion>(
    `/api/v2/templateversions/${versionId}`,
  )
  return response.data
}

export const getTemplateVersionSchema = async (
  versionId: string,
): Promise<TypesGen.ParameterSchema[]> => {
  const response = await axios.get<TypesGen.ParameterSchema[]>(
    `/api/v2/templateversions/${versionId}/schema`,
  )
  return response.data
}

export const getTemplateVersionResources = async (
  versionId: string,
): Promise<TypesGen.WorkspaceResource[]> => {
  const response = await axios.get<TypesGen.WorkspaceResource[]>(
    `/api/v2/templateversions/${versionId}/resources`,
  )
  return response.data
}

export const getTemplateVersions = async (
  templateId: string,
): Promise<TypesGen.TemplateVersion[]> => {
  const response = await axios.get<TypesGen.TemplateVersion[]>(
    `/api/v2/templates/${templateId}/versions`,
  )
  return response.data
}

export const getTemplateVersionByName = async (
  organizationId: string,
  versionName: string,
): Promise<TypesGen.TemplateVersion> => {
  const response = await axios.get<TypesGen.TemplateVersion>(
    `/api/v2/organizations/${organizationId}/templateversions/${versionName}`,
  )
  return response.data
}

export const updateTemplateMeta = async (
  templateId: string,
  data: TypesGen.UpdateTemplateMeta,
): Promise<TypesGen.Template> => {
  const response = await axios.patch<TypesGen.Template>(
    `/api/v2/templates/${templateId}`,
    data,
  )
  return response.data
}

export const deleteTemplate = async (
  templateId: string,
): Promise<TypesGen.Template> => {
  const response = await axios.delete<TypesGen.Template>(
    `/api/v2/templates/${templateId}`,
  )
  return response.data
}

export const getWorkspace = async (
  workspaceId: string,
  params?: TypesGen.WorkspaceOptions,
): Promise<TypesGen.Workspace> => {
  const response = await axios.get<TypesGen.Workspace>(
    `/api/v2/workspaces/${workspaceId}`,
    {
      params,
    },
  )
  return response.data
}

/**
 *
 * @param workspaceId
 * @returns An EventSource that emits workspace event objects (ServerSentEvent)
 */
export const watchWorkspace = (workspaceId: string): EventSource => {
  return new EventSource(
    `${location.protocol}//${location.host}/api/v2/workspaces/${workspaceId}/watch`,
    { withCredentials: true },
  )
}

interface SearchParamOptions extends TypesGen.Pagination {
  q?: string
}

export const getURLWithSearchParams = (
  basePath: string,
  options?: SearchParamOptions,
): string => {
  if (options) {
    const searchParams = new URLSearchParams()
    const keys = Object.keys(options) as (keyof SearchParamOptions)[]
    keys.forEach((key) => {
      const value = options[key]
      if (value !== undefined && value !== "") {
        searchParams.append(key, value.toString())
      }
    })
    const searchString = searchParams.toString()
    return searchString ? `${basePath}?${searchString}` : basePath
  } else {
    return basePath
  }
}

export const getWorkspaces = async (
  options: TypesGen.WorkspacesRequest,
): Promise<TypesGen.WorkspacesResponse> => {
  const url = getURLWithSearchParams("/api/v2/workspaces", options)
  const response = await axios.get<TypesGen.WorkspacesResponse>(url)
  return response.data
}

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
  )
  return response.data
}

const postWorkspaceBuild =
  (transition: WorkspaceBuildTransition) =>
  async (
    workspaceId: string,
    template_version_id?: string,
  ): Promise<TypesGen.WorkspaceBuild> => {
    const payload = {
      transition,
      template_version_id,
    }
    const response = await axios.post(
      `/api/v2/workspaces/${workspaceId}/builds`,
      payload,
    )
    return response.data
  }

export const startWorkspace = postWorkspaceBuild("start")
export const stopWorkspace = postWorkspaceBuild("stop")
export const deleteWorkspace = postWorkspaceBuild("delete")

export const cancelWorkspaceBuild = async (
  workspaceBuildId: TypesGen.WorkspaceBuild["id"],
): Promise<Types.Message> => {
  const response = await axios.patch(
    `/api/v2/workspacebuilds/${workspaceBuildId}/cancel`,
  )
  return response.data
}

export const createUser = async (
  user: TypesGen.CreateUserRequest,
): Promise<TypesGen.User> => {
  const response = await axios.post<TypesGen.User>("/api/v2/users", user)
  return response.data
}

export const createWorkspace = async (
  organizationId: string,
  userId = "me",
  workspace: TypesGen.CreateWorkspaceRequest,
): Promise<TypesGen.Workspace> => {
  const response = await axios.post<TypesGen.Workspace>(
    `/api/v2/organizations/${organizationId}/members/${userId}/workspaces`,
    workspace,
  )
  return response.data
}

export const getBuildInfo = async (): Promise<TypesGen.BuildInfoResponse> => {
  const response = await axios.get("/api/v2/buildinfo")
  return response.data
}

export const putWorkspaceAutostart = async (
  workspaceID: string,
  autostart: TypesGen.UpdateWorkspaceAutostartRequest,
): Promise<void> => {
  const payload = JSON.stringify(autostart)
  await axios.put(`/api/v2/workspaces/${workspaceID}/autostart`, payload, {
    headers: { ...CONTENT_TYPE_JSON },
  })
}

export const putWorkspaceAutostop = async (
  workspaceID: string,
  ttl: TypesGen.UpdateWorkspaceTTLRequest,
): Promise<void> => {
  const payload = JSON.stringify(ttl)
  await axios.put(`/api/v2/workspaces/${workspaceID}/ttl`, payload, {
    headers: { ...CONTENT_TYPE_JSON },
  })
}

export const updateProfile = async (
  userId: string,
  data: TypesGen.UpdateUserProfileRequest,
): Promise<TypesGen.User> => {
  const response = await axios.put(`/api/v2/users/${userId}/profile`, data)
  return response.data
}

export const activateUser = async (
  userId: TypesGen.User["id"],
): Promise<TypesGen.User> => {
  const response = await axios.put<TypesGen.User>(
    `/api/v2/users/${userId}/status/activate`,
  )
  return response.data
}

export const suspendUser = async (
  userId: TypesGen.User["id"],
): Promise<TypesGen.User> => {
  const response = await axios.put<TypesGen.User>(
    `/api/v2/users/${userId}/status/suspend`,
  )
  return response.data
}

export const deleteUser = async (
  userId: TypesGen.User["id"],
): Promise<undefined> => {
  return await axios.delete(`/api/v2/users/${userId}`)
}

// API definition:
// https://github.com/coder/coder/blob/db665e7261f3c24a272ccec48233a3e276878239/coderd/users.go#L33-L53
export const hasFirstUser = async (): Promise<boolean> => {
  try {
    // If it is success, it is true
    await axios.get("/api/v2/users/first")
    return true
  } catch (error) {
    // If it returns a 404, it is false
    if (axios.isAxiosError(error) && error.response?.status === 404) {
      return false
    }

    throw error
  }
}

export const createFirstUser = async (
  req: TypesGen.CreateFirstUserRequest,
): Promise<TypesGen.CreateFirstUserResponse> => {
  const response = await axios.post(`/api/v2/users/first`, req)
  return response.data
}

export const updateUserPassword = async (
  userId: TypesGen.User["id"],
  updatePassword: TypesGen.UpdateUserPasswordRequest,
): Promise<undefined> =>
  axios.put(`/api/v2/users/${userId}/password`, updatePassword)

export const getSiteRoles = async (): Promise<
  Array<TypesGen.AssignableRoles>
> => {
  const response = await axios.get<Array<TypesGen.AssignableRoles>>(
    `/api/v2/users/roles`,
  )
  return response.data
}

export const updateUserRoles = async (
  roles: TypesGen.Role["name"][],
  userId: TypesGen.User["id"],
): Promise<TypesGen.User> => {
  const response = await axios.put<TypesGen.User>(
    `/api/v2/users/${userId}/roles`,
    { roles },
  )
  return response.data
}

export const getUserSSHKey = async (
  userId = "me",
): Promise<TypesGen.GitSSHKey> => {
  const response = await axios.get<TypesGen.GitSSHKey>(
    `/api/v2/users/${userId}/gitsshkey`,
  )
  return response.data
}

export const regenerateUserSSHKey = async (
  userId = "me",
): Promise<TypesGen.GitSSHKey> => {
  const response = await axios.put<TypesGen.GitSSHKey>(
    `/api/v2/users/${userId}/gitsshkey`,
  )
  return response.data
}

export const getWorkspaceBuilds = async (
  workspaceId: string,
  since: Date,
): Promise<TypesGen.WorkspaceBuild[]> => {
  const response = await axios.get<TypesGen.WorkspaceBuild[]>(
    `/api/v2/workspaces/${workspaceId}/builds?since=${since.toISOString()}`,
  )
  return response.data
}

export const getWorkspaceBuildByNumber = async (
  username = "me",
  workspaceName: string,
  buildNumber: string,
): Promise<TypesGen.WorkspaceBuild> => {
  const response = await axios.get<TypesGen.WorkspaceBuild>(
    `/api/v2/users/${username}/workspace/${workspaceName}/builds/${buildNumber}`,
  )
  return response.data
}

export const getWorkspaceBuildLogs = async (
  buildname: string,
  before: Date,
): Promise<TypesGen.ProvisionerJobLog[]> => {
  const response = await axios.get<TypesGen.ProvisionerJobLog[]>(
    `/api/v2/workspacebuilds/${buildname}/logs?before=${before.getTime()}`,
  )
  return response.data
}

export const putWorkspaceExtension = async (
  workspaceId: string,
  newDeadline: dayjs.Dayjs,
): Promise<void> => {
  await axios.put(`/api/v2/workspaces/${workspaceId}/extend`, {
    deadline: newDeadline,
  })
}

export const getEntitlements = async (): Promise<TypesGen.Entitlements> => {
  try {
    const response = await axios.get("/api/v2/entitlements")
    return response.data
  } catch (error) {
    if (axios.isAxiosError(error) && error.response?.status === 404) {
      return defaultEntitlements()
    }
    throw error
  }
}

export const getAuditLogs = async (
  options: TypesGen.AuditLogsRequest,
): Promise<TypesGen.AuditLogResponse> => {
  const url = getURLWithSearchParams("/api/v2/audit", options)
  const response = await axios.get(url)
  return response.data
}

export const getAuditLogsCount = async (
  options: TypesGen.AuditLogCountRequest = {},
): Promise<TypesGen.AuditLogCountResponse> => {
  const url = getURLWithSearchParams("/api/v2/audit/count", options)
  const response = await axios.get(url)
  return response.data
}

export const getTemplateDAUs = async (
  templateId: string,
): Promise<TypesGen.TemplateDAUsResponse> => {
  const response = await axios.get(`/api/v2/templates/${templateId}/daus`)
  return response.data
}

export const getTemplateACL = async (
  templateId: string,
): Promise<TypesGen.TemplateACL> => {
  const response = await axios.get(`/api/v2/templates/${templateId}/acl`)
  return response.data
}

export const updateTemplateACL = async (
  templateId: string,
  data: TypesGen.UpdateTemplateACL,
): Promise<TypesGen.TemplateACL> => {
  const response = await axios.patch(
    `/api/v2/templates/${templateId}/acl`,
    data,
  )
  return response.data
}

export const getApplicationsHost =
  async (): Promise<TypesGen.GetAppHostResponse> => {
    const response = await axios.get(`/api/v2/applications/host`)
    return response.data
  }

export const getGroups = async (
  organizationId: string,
): Promise<TypesGen.Group[]> => {
  const response = await axios.get(
    `/api/v2/organizations/${organizationId}/groups`,
  )
  return response.data
}

export const createGroup = async (
  organizationId: string,
  data: TypesGen.CreateGroupRequest,
): Promise<TypesGen.Group> => {
  const response = await axios.post(
    `/api/v2/organizations/${organizationId}/groups`,
    data,
  )
  return response.data
}

export const getGroup = async (groupId: string): Promise<TypesGen.Group> => {
  const response = await axios.get(`/api/v2/groups/${groupId}`)
  return response.data
}

export const patchGroup = async (
  groupId: string,
  data: TypesGen.PatchGroupRequest,
): Promise<TypesGen.Group> => {
  const response = await axios.patch(`/api/v2/groups/${groupId}`, data)
  return response.data
}

export const deleteGroup = async (groupId: string): Promise<void> => {
  await axios.delete(`/api/v2/groups/${groupId}`)
}

export const getWorkspaceQuota = async (
  userID: string,
): Promise<TypesGen.WorkspaceQuota> => {
  const response = await axios.get(`/api/v2/workspace-quota/${userID}`)
  return response.data
}

export const getAgentListeningPorts = async (
  agentID: string,
): Promise<TypesGen.ListeningPortsResponse> => {
  const response = await axios.get(
    `/api/v2/workspaceagents/${agentID}/listening-ports`,
  )
  return response.data
}

export const getDeploymentConfig =
  async (): Promise<TypesGen.DeploymentConfig> => {
    const response = await axios.get(`/api/v2/config/deployment`)
    return response.data
  }

export const getReplicas = async (): Promise<TypesGen.Replica[]> => {
  const response = await axios.get(`/api/v2/replicas`)
  return response.data
}

export const getFile = async (fileId: string): Promise<ArrayBuffer> => {
  const response = await axios.get<ArrayBuffer>(`/api/v2/files/${fileId}`, {
    responseType: "arraybuffer",
  })
  return response.data
}
