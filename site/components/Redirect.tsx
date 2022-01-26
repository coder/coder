import React, { useEffect } from "react"
import { useRouter } from "next/router"

export interface RedirectProps {
  /**
   * The path to redirect to
   * @example '/projects'
   */
  to: string
}

/**
 * Helper component to perform a client-side redirect
 */
export const Redirect: React.FC<RedirectProps> = ({ to }) => {
  const router = useRouter()

  useEffect(() => {
    void router.replace(to)
  }, [])

  return null
}
