import React from "react"
import * as TypesGen from "../../api/typesGenerated"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"

export interface CreateWorkspacePageViewProps {
  loading?: boolean
  template?: TypesGen.Template
  templateVersion?: TypesGen.TemplateVersion
  error?: unknown
}

export const CreateWorkspacePageView: React.FC<CreateWorkspacePageViewProps> = () => {
  return (
    <Stack spacing={4}>
      <Margins>
        <h1>
            Create a Workspace
        </h1>
      </Margins>
    </Stack>
  )
}
