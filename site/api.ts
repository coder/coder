
interface LoginResponse {
  'session_token': string
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
    })
  })

  return await response.json()
}

export interface User {
  id: string
  email: string
  created_at: string
  username: string
}

export namespace User {
  export const current = async (): Promise<User> => {
    const response = await fetch("/api/v2/user", {
      method: "GET",
    })

    return await response.json()
  }
}