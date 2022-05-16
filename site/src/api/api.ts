import axios, { AxiosRequestHeaders } from "axios"
import { mutate } from "swr"
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

export namespace Workspace {
  export const create = async (
    organizationId: string,
    request: TypesGen.CreateWorkspaceRequest,
  ): Promise<TypesGen.Workspace> => {
    const response = await fetch(`/api/v2/organizations/${organizationId}/workspaces`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(request),
    })

    const body = await response.json()
    if (!response.ok) {
      throw new Error(body.message)
    }

    // Let SWR know that both the /api/v2/workspaces/* and /api/v2/templates/*
    // endpoints will need to fetch new data.
    const mutateWorkspacesPromise = mutate("/api/v2/workspaces")
    const mutateTemplatesPromise = mutate("/api/v2/templates")
    await Promise.all([mutateWorkspacesPromise, mutateTemplatesPromise])

    return body
  }
}

export const login = async (email: string, password: string): Promise<TypesGen.LoginWithPasswordResponse> => {
  const payload = JSON.stringify({
    email,
    password,
  })

  const response = await axios.post<TypesGen.LoginWithPasswordResponse>("/api/v2/users/login", payload, {
    headers: { ...CONTENT_TYPE_JSON },
  })

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
  const response = await axios.post<TypesGen.UserAuthorizationResponse>(`/api/v2/users/${userId}/authorization`, params)
  return response.data
}

export const getApiKey = async (): Promise<TypesGen.GenerateAPIKeyResponse> => {
  const response = await axios.post<TypesGen.GenerateAPIKeyResponse>("/api/v2/users/me/keys")
  return response.data
}

export const getUsers = async (): Promise<TypesGen.User[]> => {
  const response = await axios.get<TypesGen.User[]>("/api/v2/users?status=active")
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

export const getWorkspace = async (workspaceId: string): Promise<TypesGen.Workspace> => {
  const response = await axios.get<TypesGen.Workspace>(`/api/v2/workspaces/${workspaceId}`)
  return response.data
}

export const getWorkspaceByOwnerAndName = async (
  organizationID: string,
  username = "me",
  workspaceName: string,
): Promise<TypesGen.Workspace> => {
  const response = await axios.get<TypesGen.Workspace>(
    `/api/v2/organizations/${organizationID}/workspaces/${username}/${workspaceName}`,
  )
  return response.data
}

export const getWorkspaceResources = async (workspaceBuildID: string): Promise<TypesGen.WorkspaceResource[]> => {
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

export const createUser = async (user: TypesGen.CreateUserRequest): Promise<TypesGen.User> => {
  const response = await axios.post<TypesGen.User>("/api/v2/users", user)
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
  autostop: TypesGen.UpdateWorkspaceAutostopRequest,
): Promise<void> => {
  const payload = JSON.stringify(autostop)
  await axios.put(`/api/v2/workspaces/${workspaceID}/autostop`, payload, {
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

export const suspendUser = async (userId: TypesGen.User["id"]): Promise<TypesGen.User> => {
  const response = await axios.put<TypesGen.User>(`/api/v2/users/${userId}/suspend`)
  return response.data
}

export const updateUserPassword = async (password: string, userId: TypesGen.User["id"]): Promise<undefined> =>
  axios.put(`/api/v2/users/${userId}/password`, { password })

export const getSiteRoles = async (): Promise<Array<TypesGen.Role>> => {
  const response = await axios.get<Array<TypesGen.Role>>(`/api/v2/users/roles`)
  return response.data
}

export const updateUserRoles = async (
  roles: TypesGen.Role["name"][],
  userId: TypesGen.User["id"],
): Promise<TypesGen.User> => {
  const response = await axios.put<TypesGen.User>(`/api/v2/users/${userId}/roles`, { roles })
  return response.data
}
