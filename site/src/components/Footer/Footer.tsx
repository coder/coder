import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import { useActor } from "@xstate/react"
import React, { useContext } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { XServiceContext } from "../../xServices/StateContext"

export const Language = {
  buildInfoText: (buildInfo: TypesGen.BuildInfoResponse): string => {
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
    flex: "0",
    paddingTop: theme.spacing(2),
    paddingBottom: theme.spacing(2),
    marginTop: theme.spacing(3),
  },
  copyRight: {
    margin: theme.spacing(0.25),
  },
  buildInfo: {
    margin: theme.spacing(0.25),
  },
}))
