import React, { useEffect, useState } from "react"
import { useRouter } from "next/router"

import { FullScreenLoader } from "../Loader/FullScreenLoader"
import * as API from "./../../api"

export interface AuthenticatedRouterProps {
  fetchUser?: () => Promise<API.User>
}

export const AuthenticatedRouter: React.FC<AuthenticatedRouterProps> = ({
  children,
  fetchUser = () => API.User.current(),
}) => {
  const [isAuthenticated, setAuthenticated] = useState(false)
  const router = useRouter()

  useEffect(() => {
    const asyncFn = async () => {
      try {
        await fetchUser()
        setAuthenticated(true)
      } catch (ex) {
        router.push("/login")
      }
    }

    asyncFn()
  }, [])

  if (!isAuthenticated) {
    return <FullScreenLoader />
  }
  return <>{children}</>
}
