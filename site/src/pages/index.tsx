import { FC } from "react"
import { Navigate } from "react-router-dom"

export const IndexPage: FC = () => {
  return <Navigate to="/workspaces" replace />
}
