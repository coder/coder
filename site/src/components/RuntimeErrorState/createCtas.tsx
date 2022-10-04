import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import RefreshIcon from "@material-ui/icons/Refresh"
import { ReactElement } from "react"
import { CopyButton } from "../CopyButton/CopyButton"

export const Language = {
  reloadApp: "Reload Application",
  copyReport: "Copy Report",
}

/**
 * A wrapper component for a full-width copy button
 */
const CopyStackButton = ({ text }: { text: string }): ReactElement => {
  const styles = useStyles()

  return (
    <CopyButton
      text={text}
      ctaCopy={Language.copyReport}
      wrapperClassName={styles.buttonWrapper}
      buttonClassName={styles.copyButton}
    />
  )
}

/**
 * A button that reloads our application
 */
const ReloadAppButton = (): ReactElement => {
  const styles = useStyles()

  return (
    <Button
      className={styles.buttonWrapper}
      variant="outlined"
      color="primary"
      startIcon={<RefreshIcon />}
      onClick={() => location.replace("/")}
    >
      {Language.reloadApp}
    </Button>
  )
}

/**
 * createCtas generates an array of buttons to be used with our error boundary UI
 */
export const createCtas = (codeBlock: string[]): ReactElement[] => {
  return [
    <CopyStackButton key="copy-stack-btn" text={codeBlock.join("\r\n")} />,
    <ReloadAppButton key="reload-app-btn" />,
  ]
}

const useStyles = makeStyles((theme) => ({
  buttonWrapper: {
    marginTop: theme.spacing(1),
    marginLeft: 0,
    flex: theme.spacing(1),
    textTransform: "uppercase",
    fontSize: theme.typography.fontSize,
  },

  copyButton: {
    width: "100%",
    marginRight: theme.spacing(1),
    backgroundColor: theme.palette.primary.main,
    textTransform: "uppercase",
    fontSize: theme.typography.fontSize,
  },
}))
