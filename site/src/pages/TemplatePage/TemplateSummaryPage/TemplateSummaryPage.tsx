import { useQuery } from "@tanstack/react-query"
import {
  getTemplateVersion,
  getTemplateVersionResources,
  getTemplateVersions,
  getTemplateDAUs,
} from "api/api"
import { useTemplateLayoutContext } from "components/TemplateLayout/TemplateLayout"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { TemplateSummaryPageView } from "./TemplateSummaryPageView"

const fetchTemplateSummary = async (
  templateId: string,
  activeVersionId: string,
) => {
  const [activeVersion, resources, versions, daus] = await Promise.all([
    getTemplateVersion(activeVersionId),
    getTemplateVersionResources(activeVersionId),
    getTemplateVersions(templateId),
    getTemplateDAUs(templateId),
  ])

  return {
    activeVersion,
    resources,
    versions,
    daus,
  }
}

const useTemplateSummaryData = (
  templateId: string,
  activeVersionId: string,
) => {
  return useQuery({
    queryKey: ["template", templateId, "summary"],
    queryFn: () => fetchTemplateSummary(templateId, activeVersionId),
  })
}

export const TemplateSummaryPage: FC = () => {
  const { template } = useTemplateLayoutContext()
  const { data } = useTemplateSummaryData(
    template.id,
    template.active_version_id,
  )

  return (
    <>
      <Helmet>
        <title>
          {pageTitle(
            `${
              template.display_name.length > 0
                ? template.display_name
                : template.name
            } · Template`,
          )}
        </title>
      </Helmet>
      <TemplateSummaryPageView data={data} template={template} />
    </>
  )
}

export default TemplateSummaryPage
