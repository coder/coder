import { useQuery } from "@tanstack/react-query"
import { getTemplateVersions } from "api/api"
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import { useTemplateLayoutContext } from "components/TemplateLayout/TemplateLayout"
import { VersionsTable } from "components/VersionsTable/VersionsTable"
import { useState } from "react"
import { Helmet } from "react-helmet-async"
import { getTemplatePageTitle } from "../utils"

const TemplateVersionsPage = () => {
  const { template, permissions } = useTemplateLayoutContext()
  const { data } = useQuery({
    queryKey: ["template", "versions", template.id],
    queryFn: () => getTemplateVersions(template.id),
  })
  const [promoteState, setPromoteState] = useState<
    "idle" | "confirming" | "promoting"
  >("idle")

  return (
    <>
      <Helmet>
        <title>{getTemplatePageTitle("Versions", template)}</title>
      </Helmet>
      <VersionsTable
        versions={data}
        onPromoteClick={
          permissions.canUpdateTemplate
            ? () => setPromoteState("confirming")
            : undefined
        }
        activeVersionId={template.active_version_id}
      />
      <ConfirmDialog
        type="info"
        hideCancel={false}
        open={promoteState !== "idle"}
        onConfirm={() => {}}
        onClose={() => setPromoteState("idle")}
        title="Promote version"
        confirmLoading={promoteState === "promoting"}
        confirmText="Promote"
        description="Are you sure you want to promote this version?"
      />
    </>
  )
}

export default TemplateVersionsPage
