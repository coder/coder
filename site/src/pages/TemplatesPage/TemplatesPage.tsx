import { useMachine } from "@xstate/react"
import { useEntitlements } from "hooks/useEntitlements"
import { useOrganizationId } from "hooks/useOrganizationId"
import { usePermissions } from "hooks/usePermissions"
import React from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "../../util/page"
import { templatesMachine } from "../../xServices/templates/templatesXService"
import { TemplatesPageView } from "./TemplatesPageView"

export const TemplatesPage: React.FC = () => {
  const organizationId = useOrganizationId()
  const permissions = usePermissions()
  const entitlements = useEntitlements()
  const [templatesState] = useMachine(templatesMachine, {
    context: {
      organizationId,
      permissions,
    },
  })

  return (
    <>
      <Helmet>
        <title>{pageTitle("Templates")}</title>
      </Helmet>
      <TemplatesPageView
        context={templatesState.context}
        entitlements={entitlements}
      />
    </>
  )
}

export default TemplatesPage
