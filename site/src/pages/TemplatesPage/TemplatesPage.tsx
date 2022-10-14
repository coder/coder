import { useActor, useMachine } from "@xstate/react"
import React, { useContext } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "../../util/page"
import { XServiceContext } from "../../xServices/StateContext"
import { templatesMachine } from "../../xServices/templates/templatesXService"
import { TemplatesPageView } from "./TemplatesPageView"

export const TemplatesPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [authState] = useActor(xServices.authXService)
  const [templatesState] = useMachine(templatesMachine)
  const { templates, getOrganizationsError, getTemplatesError } =
    templatesState.context

  return (
    <>
      <Helmet>
        <title>{pageTitle("Templates")}</title>
      </Helmet>
      <TemplatesPageView
        templates={templates}
        canCreateTemplate={authState.context.permissions?.createTemplates}
        loading={templatesState.hasTag("loading")}
        getOrganizationsError={getOrganizationsError}
        getTemplatesError={getTemplatesError}
      />
    </>
  )
}

export default TemplatesPage
