import axios, { AxiosRequestHeaders } from "axios"
import { mutate } from "swr"

const CONTENT_TYPE_JSON: AxiosRequestHeaders = {
  "Content-Type": "application/json",
}

/**
 * `Organization` must be kept in sync with the go struct in organizations.go
 */
export interface Organization {
  id: string
  name: string
  created_at: string
  updated_at: string
}

export interface Provisioner {
  id: string
  name: string
}

export const provisioners: Provisioner[] = [
  {
    id: "terraform",
    name: "Terraform",
  },
  {
    id: "cdr-basic",
    name: "Basic",
  },
]

// This must be kept in sync with the `Project` struct in the back-end
export interface Project {
  id: string
  created_at: string
  updated_at: string
  organization_id: string
  name: string
  provisioner: string
  active_version_id: string
}

export interface CreateProjectRequest {
  name: string
  organizationId: string
  provisioner: string
}

export namespace Project {
  export const create = async (request: CreateProjectRequest): Promise<Project> => {
    const response = await fetch(`/api/v2/projects/${request.organizationId}/`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(request),
    })

    const body = await response.json()
    await mutate("/api/v2/projects")
    if (!response.ok) {
      throw new Error(body.message)
    }

    return body
  }
}

export interface CreateWorkspaceRequest {
  name: string
  project_id: string
}

// Must be kept in sync with backend Workspace struct
export interface Workspace {
  id: string
  created_at: string
  updated_at: string
  owner_id: string
  project_id: string
  name: string
}

export namespace Workspace {
  export const create = async (request: CreateWorkspaceRequest): Promise<Workspace> => {
    const response = await fetch(`/api/v2/users/me/workspaces`, {
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

    // Let SWR know that both the /api/v2/workspaces/* and /api/v2/projects/*
    // endpoints will need to fetch new data.
    const mutateWorkspacesPromise = mutate("/api/v2/workspaces")
    const mutateProjectsPromise = mutate("/api/v2/projects")
    await Promise.all([mutateWorkspacesPromise, mutateProjectsPromise])

    return body
  }
}

export interface LoginResponse {
  session_token: string
}

export const login = async (email: string, password: string): Promise<LoginResponse> => {
  const payload = JSON.stringify({
    email,
    password,
  })

  const response = await axios.post<LoginResponse>("/api/v2/users/login", payload, {
    headers: { ...CONTENT_TYPE_JSON },
  })

  return response.data
}

export const logout = async (): Promise<void> => {
  await axios.post("/api/v2/users/logout")
}

export interface APIKeyResponse {
  key: string
}

export const getApiKey = async (): Promise<APIKeyResponse> => {
  const response = await axios.post<APIKeyResponse>("/api/v2/users/me/keys")
  return response.data
}
