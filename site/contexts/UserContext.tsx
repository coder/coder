import { useRouter } from "next/router"
import React, { useContext, useEffect } from "react"
import useSWR from "swr"

import * as API from "../api"

export interface User {
  readonly id: string
  readonly username: string
  readonly email: string
  readonly created_at: string
}

export interface UserContext {
  readonly error?: Error
  readonly me?: User
  readonly signOut: () => Promise<void>
}

const UserContext = React.createContext<UserContext>({
  signOut: () => {
    return Promise.reject("Sign out API not available")
  },
})

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
  const router = useRouter()
  const { data, error, mutate } = useSWR("/api/v2/user")

  const signOut = async () => {
    await API.logout()
    // Tell SWR to invalidate the cache for the user endpoint
    mutate("/api/v2/user")
    router.push({
      pathname: "/login",
      query: {
        redirect: router.asPath,
      },
    })
  }

  return (
    <UserContext.Provider
      value={{
        error: error,
        me: data,
        signOut: signOut,
      }}
    >
      {props.children}
    </UserContext.Provider>
  )
}
