import { FC } from "react"
import { Navigate } from "react-router-dom"

export const IndexPage: FC<React.PropsWithChildren<unknown>> = () => {
  return <Navigate to="/workspaces" replace />
}
