import React, { useEffect, useState } from "react"
import { useRouter } from "next/router"

import { FullScreenLoader } from "../Loader/FullScreenLoader"
import * as API from "./../../api"

export const AuthenticatedRouter: React.FC = ({ children }) => {
  const [isAuthenticated, setAuthenticated] = useState(false)
  const router = useRouter()

  useEffect(() => {
    const asyncFn = async () => {
      try {
        await API.User.current()
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
