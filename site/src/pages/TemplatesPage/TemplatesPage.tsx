import { useActor, useMachine } from "@xstate/react"
import React, { useContext } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "../../util/page"
import { XServiceContext } from "../../xServices/StateContext"
import { templatesMachine } from "../../xServices/templates/templatesXService"
import { TemplatesPageView } from "./TemplatesPageView"

const TemplatesPage: React.FC<React.PropsWithChildren<unknown>> = () => {
  const xServices = useContext(XServiceContext)
  const [authState] = useActor(xServices.authXService)
  const [templatesState] = useMachine(templatesMachine)

  return (
    <>
      <Helmet>
        <title>{pageTitle("Templates")}</title>
      </Helmet>
      <TemplatesPageView
        templates={templatesState.context.templates}
        canCreateTemplate={authState.context.permissions?.createTemplates}
        loading={templatesState.hasTag("loading")}
      />
    </>
  )
}

export default TemplatesPage
