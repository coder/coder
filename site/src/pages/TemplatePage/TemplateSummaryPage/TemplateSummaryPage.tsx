import { useTemplateLayoutContext } from "components/TemplateLayout/TemplateLayout"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { getTemplatePageTitle } from "../utils"
import { useTemplateSummaryData } from "./data"
import { TemplateSummaryPageView } from "./TemplateSummaryPageView"

export const TemplateSummaryPage: FC = () => {
  const { template, activeVersion } = useTemplateLayoutContext()
  const { data } = useTemplateSummaryData(
    template.id,
    template.active_version_id,
  )

  return (
    <>
      <Helmet>
        <title>{getTemplatePageTitle("Template", template)}</title>
      </Helmet>
      <TemplateSummaryPageView
        data={data}
        template={template}
        activeVersion={activeVersion}
      />
    </>
  )
}

export default TemplateSummaryPage
