import React, { useEffect } from "react"
import { useRouter } from "next/router"

export interface RedirectProps {
  to: string
}

export const Redirect: React.FC<RedirectProps> = ({ to }) => {
  const router = useRouter()

  useEffect(() => {
    void router.replace(to)
  }, [])

  return null
}