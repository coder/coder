import { mutate } from "swr"

interface LoginResponse {
  session_token: string
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

export const login = async (email: string, password: string): Promise<LoginResponse> => {
  const response = await fetch("/api/v2/login", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      email,
      password,
    }),
  })

  const body = await response.json()
  if (!response.ok) {
    throw new Error(body.message)
  }

  return body
}

export const logout = async (): Promise<void> => {
  const response = await fetch("/api/v2/logout", {
    method: "POST",
  })

  if (!response.ok) {
    const body = await response.json()
    throw new Error(body.message)
  }

  return
}
