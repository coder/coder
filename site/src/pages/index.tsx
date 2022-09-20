import { FC } from "react"
import { Navigate, useSearchParams } from "react-router-dom"

export const IndexPage: FC = () => {
  const [searchParams, _] = useSearchParams()
  const template = searchParams.get("template")

  if (template) {
    return <Navigate to={`/templates/${template}`} replace />
  }

  return <Navigate to="/workspaces" replace />
}
