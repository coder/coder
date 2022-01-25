interface LoginResponse {
  session_token: string
}

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
