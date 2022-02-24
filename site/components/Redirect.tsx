import React, { useEffect } from "react"
import { useNavigate } from "react-router-dom"

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
  const navigate = useNavigate()

  useEffect(() => {
    void navigate(to, { replace: true })
  }, [navigate, to])

  return null
}
