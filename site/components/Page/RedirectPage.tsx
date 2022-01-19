import React, { useEffect } from "react"
import { useRouter } from "next/router"

export interface RedirectPageProps {
  path: string
}

export const RedirectPage: React.FC<RedirectPageProps> = ({ path }) => {
  const router = useRouter()

  useEffect(() => {
    router.push(path)
  })

  return null
}
