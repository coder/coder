import { useTemplateLayoutContext } from "components/TemplateLayout/TemplateLayout"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { TemplateSummaryPageView } from "./TemplateSummaryPageView"

export const TemplateSummaryPage: FC = () => {
  const { template, activeVersion, resources, versions, daus } =
    useTemplateLayoutContext()

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
        template={template}
        activeTemplateVersion={activeVersion}
        templateResources={resources}
        templateVersions={versions}
        templateDAUs={daus}
      />
    </>
  )
}

export default TemplateSummaryPage
