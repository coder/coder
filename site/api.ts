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

  return await readOrThrowResponse(response)
}

export const logout = async (): Promise<void> => {
  const response = await fetch("/api/v2/logout", {
    method: "POST",
  })

  return await readOrThrowResponse(response)
}

const readOrThrowResponse = async (response: Response): Promise<any> => {
  const body = await response.json()
  if (!response.ok) {
    throw new Error(body.message)
  }

  return body
}
