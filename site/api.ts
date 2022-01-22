interface LoginResponse {
  session_token: string
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
