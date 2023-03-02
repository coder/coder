import { useTemplateLayoutContext } from "components/TemplateLayout/TemplateLayout"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { TemplateSummaryPageView } from "./TemplateSummaryPageView"
import { Loader } from "components/Loader/Loader"

export const TemplateSummaryPage: FC = () => {
  const { context } = useTemplateLayoutContext()
  const {
    template,
    activeTemplateVersion,
    templateResources,
    templateVersions,
    templateDAUs,
  } = context

  if (!template || !activeTemplateVersion || !templateResources) {
    return <Loader />
  }

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
        activeTemplateVersion={activeTemplateVersion}
        templateResources={templateResources}
        templateVersions={templateVersions}
        templateDAUs={templateDAUs}
      />
    </>
  )
}

export default TemplateSummaryPage
