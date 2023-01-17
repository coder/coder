import { useActor } from "@xstate/react"
import { useContext, useEffect, useState, FC, PropsWithChildren } from "react"
import { Helmet } from "react-helmet-async"
import { getApiKey } from "../../api/api"
import { pageTitle } from "../../util/page"
import { XServiceContext } from "../../xServices/StateContext"
import { CliAuthPageView } from "./CliAuthPageView"

export const CliAuthenticationPage: FC<PropsWithChildren<unknown>> = () => {
  const xServices = useContext(XServiceContext)
  const [authState] = useActor(xServices.authXService)
  const { me } = authState.context
  const [apiKey, setApiKey] = useState<string | null>(null)

  useEffect(() => {
    if (me?.id) {
      void getApiKey().then(({ key }) => {
        setApiKey(key)
      })
    }
  }, [me?.id])

  return (
    <>
      <Helmet>
        <title>{pageTitle("CLI Auth")}</title>
      </Helmet>
      <CliAuthPageView sessionToken={apiKey} />
    </>
  )
}

export default CliAuthenticationPage
