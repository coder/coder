import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import { BuildInfoResponse } from "../../api/types"
import { XServiceContext } from "../../xServices/StateContext"

export const Language = {
  buildInfoText: (buildInfo: BuildInfoResponse): string => {
    return `Coder ${buildInfo.version}`
  },
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
        <div className={styles.buildInfo}>
          <Link variant="caption" target="_blank" href={buildInfoState.context.buildInfo.external_url}>
            {Language.buildInfoText(buildInfoState.context.buildInfo)}
          </Link>
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
    margin: theme.spacing(0.25),
  },
  buildInfo: {
    margin: theme.spacing(0.25),
  },
}))
