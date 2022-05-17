import React from "react"
import { Navigate } from "react-router-dom"

export const IndexPage: React.FC = () => {
  return <Navigate to="/workspaces" replace />
}
