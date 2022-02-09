import React from "react"

import * as API from "../../api"

export interface WorkspaceProps {
  workspace: API.Workspace
}

/**
 * Workspace is the top-level component for viewing an individual workspace
 */
export const Workspace: React.FC<WorkspaceProps> = ({ workspace }) => {
  return <div>Hello, workspace: <span>{workspace.name}</span></div>
}