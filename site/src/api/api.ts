import axios, { AxiosRequestHeaders } from "axios"
import dayjs from "dayjs"
import * as Types from "./types"
import { WorkspaceBuildTransition } from "./types"
import * as TypesGen from "./typesGenerated"

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
  const response = await axios.get<TypesGen.AuthMethods>("/api/v2/users/authmethods")
  return response.data
}

export const checkUserPermissions = async (
  userId: string,
  params: TypesGen.UserAuthorizationRequest,
): Promise<TypesGen.UserAuthorizationResponse> => {
  const response = await axios.post<TypesGen.UserAuthorizationResponse>(
    `/api/v2/users/${userId}/authorization`,
    params,
  )
  return response.data
}

export const getApiKey = async (): Promise<TypesGen.GenerateAPIKeyResponse> => {
  const response = await axios.post<TypesGen.GenerateAPIKeyResponse>("/api/v2/users/me/keys")
  return response.data
}

export const getUsers = async (filter?: TypesGen.UsersRequest): Promise<TypesGen.User[]> => {
  const url = getURLWithSearchParams("/api/v2/users", filter)
  const response = await axios.get<TypesGen.User[]>(url)
  return response.data
}

export const getOrganization = async (organizationId: string): Promise<TypesGen.Organization> => {
  const response = await axios.get<TypesGen.Organization>(`/api/v2/organizations/${organizationId}`)
  return response.data
}

export const getOrganizations = async (): Promise<TypesGen.Organization[]> => {
  const response = await axios.get<TypesGen.Organization[]>("/api/v2/users/me/organizations")
  return response.data
}

export const getTemplate = async (templateId: string): Promise<TypesGen.Template> => {
  const response = await axios.get<TypesGen.Template>(`/api/v2/templates/${templateId}`)
  return response.data
}

export const getTemplates = async (organizationId: string): Promise<TypesGen.Template[]> => {
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

export const getTemplateVersion = async (versionId: string): Promise<TypesGen.TemplateVersion> => {
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

export const updateTemplateMeta = async (
  templateId: string,
  data: TypesGen.UpdateTemplateMeta,
): Promise<TypesGen.Template> => {
  const response = await axios.patch<TypesGen.Template>(`/api/v2/templates/${templateId}`, data)
  return response.data
}

export const getWorkspace = async (
  workspaceId: string,
  params?: TypesGen.WorkspaceOptions,
): Promise<TypesGen.Workspace> => {
  const response = await axios.get<TypesGen.Workspace>(`/api/v2/workspaces/${workspaceId}`, {
    params,
  })
  return response.data
}

export const getURLWithSearchParams = (
  basePath: string,
  filter?: TypesGen.WorkspaceFilter | TypesGen.UsersRequest,
): string => {
  const searchParams = new URLSearchParams()

  if (filter?.q && filter.q !== "") {
    searchParams.append("q", filter.q)
  }

  const searchString = searchParams.toString()

  return searchString ? `${basePath}?${searchString}` : basePath
}

export const getWorkspaces = async (
  filter?: TypesGen.WorkspaceFilter,
): Promise<TypesGen.Workspace[]> => {
  const url = getURLWithSearchParams("/api/v2/workspaces", filter)
  const response = await axios.get<TypesGen.Workspace[]>(url)
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

export const getWorkspaceResources = async (
  workspaceBuildID: string,
): Promise<TypesGen.WorkspaceResource[]> => {
  const response = await axios.get<TypesGen.WorkspaceResource[]>(
    `/api/v2/workspacebuilds/${workspaceBuildID}/resources`,
  )
  return response.data
}

const postWorkspaceBuild =
  (transition: WorkspaceBuildTransition) =>
  async (workspaceId: string, template_version_id?: string): Promise<TypesGen.WorkspaceBuild> => {
    const payload = {
      transition,
      template_version_id,
    }
    const response = await axios.post(`/api/v2/workspaces/${workspaceId}/builds`, payload)
    return response.data
  }

export const startWorkspace = postWorkspaceBuild("start")
export const stopWorkspace = postWorkspaceBuild("stop")
export const deleteWorkspace = postWorkspaceBuild("delete")

export const cancelWorkspaceBuild = async (
  workspaceBuildId: TypesGen.WorkspaceBuild["id"],
): Promise<Types.Message> => {
  const response = await axios.patch(`/api/v2/workspacebuilds/${workspaceBuildId}/cancel`)
  return response.data
}

export const createUser = async (user: TypesGen.CreateUserRequest): Promise<TypesGen.User> => {
  const response = await axios.post<TypesGen.User>("/api/v2/users", user)
  return response.data
}

export const createWorkspace = async (
  organizationId: string,
  workspace: TypesGen.CreateWorkspaceRequest,
): Promise<TypesGen.Workspace> => {
  const response = await axios.post<TypesGen.Workspace>(
    `/api/v2/organizations/${organizationId}/workspaces`,
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

export const activateUser = async (userId: TypesGen.User["id"]): Promise<TypesGen.User> => {
  const response = await axios.put<TypesGen.User>(`/api/v2/users/${userId}/status/activate`)
  return response.data
}

export const suspendUser = async (userId: TypesGen.User["id"]): Promise<TypesGen.User> => {
  const response = await axios.put<TypesGen.User>(`/api/v2/users/${userId}/status/suspend`)
  return response.data
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
): Promise<undefined> => axios.put(`/api/v2/users/${userId}/password`, updatePassword)

export const getSiteRoles = async (): Promise<Array<TypesGen.AssignableRoles>> => {
  const response = await axios.get<Array<TypesGen.AssignableRoles>>(`/api/v2/users/roles`)
  return response.data
}

export const updateUserRoles = async (
  roles: TypesGen.Role["name"][],
  userId: TypesGen.User["id"],
): Promise<TypesGen.User> => {
  const response = await axios.put<TypesGen.User>(`/api/v2/users/${userId}/roles`, { roles })
  return response.data
}

export const getUserSSHKey = async (userId = "me"): Promise<TypesGen.GitSSHKey> => {
  const response = await axios.get<TypesGen.GitSSHKey>(`/api/v2/users/${userId}/gitsshkey`)
  return response.data
}

export const regenerateUserSSHKey = async (userId = "me"): Promise<TypesGen.GitSSHKey> => {
  const response = await axios.put<TypesGen.GitSSHKey>(`/api/v2/users/${userId}/gitsshkey`)
  return response.data
}

export const getWorkspaceBuilds = async (
  workspaceId: string,
): Promise<TypesGen.WorkspaceBuild[]> => {
  const response = await axios.get<TypesGen.WorkspaceBuild[]>(
    `/api/v2/workspaces/${workspaceId}/builds`,
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
  await axios.put(`/api/v2/workspaces/${workspaceId}/extend`, { deadline: newDeadline })
}

export const getEntitlements = async (): Promise<TypesGen.Entitlements> => {
  const response = await axios.get("/api/v2/entitlements")
  return response.data
}

export const getDAUs = async (): Promise<TypesGen.GetDAUsResponse> => {
  const response = await axios.get("/api/v2/metrics/daus")
  return response.data
}
