import { useRouter } from "next/router"
import React, { useContext, useEffect } from "react"
import useSWR from "swr"

export interface User {
  readonly id: string
  readonly username: string
  readonly email: string
  readonly created_at: string
}

export interface UserContext {
  readonly error?: Error
  readonly me?: User
}

const UserContext = React.createContext<UserContext>({})

export const useUser = (redirectOnError = false): UserContext => {
  const ctx = useContext(UserContext)
  const router = useRouter()

  const requestError = ctx.error
  useEffect(() => {
    if (redirectOnError && requestError) {
      router.push({
        pathname: "/login",
        query: {
          redirect: router.asPath,
        },
      })
    }
  }, [redirectOnError, requestError])

  return ctx
}

export const UserProvider: React.FC = (props) => {
  const { data, error } = useSWR("/api/v2/user")

  return (
    <UserContext.Provider
      value={{
        error: error,
        me: data,
      }}
    >
      {props.children}
    </UserContext.Provider>
  )
}
