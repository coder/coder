import { useQuery } from "@tanstack/react-query"
import { getTemplateVersions } from "api/api"
import { useTemplateLayoutContext } from "components/TemplateLayout/TemplateLayout"
import { VersionsTable } from "components/VersionsTable/VersionsTable"
import { Helmet } from "react-helmet-async"
import { getTemplatePageTitle } from "../utils"

const TemplateVersionsPage = () => {
  const { template } = useTemplateLayoutContext()
  const { data } = useQuery({
    queryKey: ["template", "versions", template.id],
    queryFn: () => getTemplateVersions(template.id),
  })

  return (
    <>
      <Helmet>
        <title>{getTemplatePageTitle("Versions", template)}</title>
      </Helmet>
      <VersionsTable
        versions={data}
        activeVersionId={template.active_version_id}
      />
    </>
  )
}

export default TemplateVersionsPage
