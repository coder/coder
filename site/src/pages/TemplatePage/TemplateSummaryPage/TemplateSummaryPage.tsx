import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useOutletContext } from "react-router-dom"
import { pageTitle } from "util/page"
import { TemplateContext } from "xServices/template/templateXService"
import { TemplateSummaryPageView } from "./TemplateSummaryPageView"

export const TemplateSummaryPage: FC<React.PropsWithChildren<unknown>> = () => {
  const {
    template,
    activeTemplateVersion,
    templateResources,
    templateVersions,
    deleteTemplateError,
    templateDAUs,
  } = useOutletContext<TemplateContext>()

  if (!template || !activeTemplateVersion || !templateResources) {
    throw new Error(
      "This page should not be displayed until template, activeTemplateVersion or templateResources being loaded.",
    )
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${template.name} · Template`)}</title>
      </Helmet>
      <TemplateSummaryPageView
        template={template}
        activeTemplateVersion={activeTemplateVersion}
        templateResources={templateResources}
        templateVersions={templateVersions}
        templateDAUs={templateDAUs}
        deleteTemplateError={deleteTemplateError}
      />
    </>
  )
}

export default TemplateSummaryPage
