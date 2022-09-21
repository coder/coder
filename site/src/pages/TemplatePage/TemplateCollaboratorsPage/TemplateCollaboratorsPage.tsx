import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useOutletContext } from "react-router-dom"
import { pageTitle } from "util/page"
import { TemplateContext } from "xServices/template/templateXService"
import { TemplateCollaboratorsPageView } from "./TemplateCollaboratorsPageView"

export const TemplateCollaboratorsPage: FC<React.PropsWithChildren<unknown>> = () => {
  const { template, activeTemplateVersion, templateResources, deleteTemplateError } =
    useOutletContext<TemplateContext>()

  if (!template || !activeTemplateVersion || !templateResources) {
    throw new Error(
      "This page should not be displayed until template, activeTemplateVersion or templateResources being loaded.",
    )
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${template.name} · Collaborators`)}</title>
      </Helmet>
      <TemplateCollaboratorsPageView deleteTemplateError={deleteTemplateError} />
    </>
  )
}

export default TemplateCollaboratorsPage
