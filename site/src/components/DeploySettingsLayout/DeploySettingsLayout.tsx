import { makeStyles } from "@material-ui/core/styles"
import { Margins } from "components/Margins/Margins"
import { Stack } from "components/Stack/Stack"
import { Sidebar } from "./Sidebar"
import React, {
  createContext,
  PropsWithChildren,
  useContext,
  useEffect,
} from "react"
import { useActor } from "@xstate/react"
import { XServiceContext } from "xServices/StateContext"
import { Loader } from "components/Loader/Loader"
import { DeploymentFlags } from "api/typesGenerated"

type DeploySettingsContextValue = { deploymentFlags: DeploymentFlags }

const DeploySettingsContext = createContext<
  DeploySettingsContextValue | undefined
>(undefined)

export const useDeploySettings = (): DeploySettingsContextValue => {
  const context = useContext(DeploySettingsContext)
  if (!context) {
    throw new Error(
      "useDeploySettings should be used inside of DeploySettingsLayout",
    )
  }
  return context
}

export const DeploySettingsLayout: React.FC<PropsWithChildren> = ({
  children,
}) => {
  const xServices = useContext(XServiceContext)
  const [state, send] = useActor(xServices.deploymentFlagsXService)
  const styles = useStyles()
  const { deploymentFlags } = state.context

  useEffect(() => {
    if (state.matches("idle")) {
      send("LOAD")
    }
  }, [send, state])

  return (
    <Margins>
      <Stack className={styles.wrapper} direction="row" spacing={5}>
        <Sidebar />
        <main className={styles.content}>
          {deploymentFlags ? (
            <DeploySettingsContext.Provider value={{ deploymentFlags }}>
              {children}
            </DeploySettingsContext.Provider>
          ) : (
            <Loader />
          )}
        </main>
      </Stack>
    </Margins>
  )
}

const useStyles = makeStyles((theme) => ({
  wrapper: {
    padding: theme.spacing(6, 0),
  },

  content: {
    maxWidth: 800,
    width: "100%",
  },
}))
