import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { BuildInfoResponse } from "../../api/types"
import { XServiceContext } from "../../xServices/StateContext"

export const Language = {
  buildInfoText: (buildInfo: BuildInfoResponse) => {
    return `Coder ${buildInfo.version}`
  }
}

export const Footer: React.FC = ({ children }) => {
  const styles = useFooterStyles()
  const xServices = useContext(XServiceContext)
  const [buildInfoState] = useActor(xServices.buildInfoXService)

  return (
    <div className={styles.root}>
      {children}
      <div className={styles.copyRight}>
        <Typography color="textSecondary" variant="caption">
          {`Copyright \u00a9 ${new Date().getFullYear()} Coder Technologies, Inc. All rights reserved.`}
        </Typography>
      </div>
      {buildInfoState.context.buildInfo && (
        <div className={styles.version}>
          <Typography color="textSecondary" variant="caption">
            {Language.buildInfoText(buildInfoState.context.buildInfo)}
          </Typography>
        </div>
      )}
    </div>
  )
}

const useFooterStyles = makeStyles((theme) => ({
  root: {
    textAlign: "center",
    marginBottom: theme.spacing(5),
    flex: "0",
  },
  copyRight: {
    backgroundColor: theme.palette.background.default,
    margin: theme.spacing(0.25),
  },
  version: {
    backgroundColor: theme.palette.background.default,
    margin: theme.spacing(0.25),
  },
}))
