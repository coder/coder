import { Button } from "@material-ui/core"
import { makeStyles } from "@material-ui/core/styles"
import { useMachine } from "@xstate/react"
import React from "react"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { workspacesMachine } from "../../xServices/workspaces/workspacesXService"

export const Language = {
  title: "Workspaces",
}

export const WorkspacesPage: React.FC = () => {
  const styles = useStyles()
  const [workspacesState] = useMachine(workspacesMachine)

  console.log(workspacesState.context.workspaces)

  return (
    <Stack spacing={4}>
      <Margins>
        <img className={styles.boxes} alt="boxes" src="/boxes.png" />
        <Button>test</Button>
      </Margins>
    </Stack>
  )
}

const useStyles = makeStyles(() => ({
  boxes: {
    position: "absolute",
  },
}))
