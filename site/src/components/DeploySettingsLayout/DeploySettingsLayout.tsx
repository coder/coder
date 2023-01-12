import { makeStyles } from "@material-ui/core/styles"
import { Margins } from "components/Margins/Margins"
import { Stack } from "components/Stack/Stack"
import { Sidebar } from "./Sidebar"
import {
  createContext,
  PropsWithChildren,
  Suspense,
  useContext,
  FC,
} from "react"
import { useMachine } from "@xstate/react"
import { Loader } from "components/Loader/Loader"
import { DeploymentConfig } from "api/typesGenerated"
import { deploymentConfigMachine } from "xServices/deploymentConfig/deploymentConfigMachine"

type DeploySettingsContextValue = { deploymentConfig: DeploymentConfig }

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

export const DeploySettingsLayout: FC<PropsWithChildren> = ({ children }) => {
  const [state] = useMachine(deploymentConfigMachine)
  const styles = useStyles()
  const { deploymentConfig } = state.context

  return (
    <Margins>
      <Stack className={styles.wrapper} direction="row" spacing={6}>
        <Sidebar />
        <main className={styles.content}>
          {deploymentConfig ? (
            <DeploySettingsContext.Provider
              value={{ deploymentConfig: deploymentConfig }}
            >
              <Suspense fallback={<Loader />}>{children}</Suspense>
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
