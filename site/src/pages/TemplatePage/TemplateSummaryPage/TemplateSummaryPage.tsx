import { useTemplateLayoutContext } from "components/TemplateLayout/TemplateLayout"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
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
      <TemplateSummaryPageView
        data={data}
        template={template}
        activeVersion={activeVersion}
      />
    </>
  )
}

export default TemplateSummaryPage
