import IconButton from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import Tooltip from "@material-ui/core/Tooltip"
import Check from "@material-ui/icons/Check"
import { useClipboard } from "hooks/useClipboard"
import { combineClasses } from "../../util/combineClasses"
import { FileCopyIcon } from "../Icons/FileCopyIcon"

interface CopyButtonProps {
  text: string
  ctaCopy?: string
  wrapperClassName?: string
  buttonClassName?: string
  tooltipTitle?: string
}

export const Language = {
  tooltipTitle: "Copy to clipboard",
  ariaLabel: "Copy to clipboard",
}

/**
 * Copy button used inside the CodeBlock component internally
 */
export const CopyButton: React.FC<React.PropsWithChildren<CopyButtonProps>> = ({
  text,
  ctaCopy,
  wrapperClassName = "",
  buttonClassName = "",
  tooltipTitle = Language.tooltipTitle,
}) => {
  const styles = useStyles()
  const { isCopied, copy: copyToClipboard } = useClipboard(text)

  return (
    <Tooltip title={tooltipTitle} placement="top">
      <div
        className={combineClasses([styles.copyButtonWrapper, wrapperClassName])}
      >
        <IconButton
          className={combineClasses([styles.copyButton, buttonClassName])}
          onClick={copyToClipboard}
          size="small"
          aria-label={Language.ariaLabel}
        >
          {isCopied ? (
            <Check className={styles.fileCopyIcon} />
          ) : (
            <FileCopyIcon className={styles.fileCopyIcon} />
          )}
          {ctaCopy && <div className={styles.buttonCopy}>{ctaCopy}</div>}
        </IconButton>
      </div>
    </Tooltip>
  )
}

const useStyles = makeStyles((theme) => ({
  copyButtonWrapper: {
    display: "flex",
  },
  copyButton: {
    borderRadius: theme.shape.borderRadius,
    padding: theme.spacing(0.85),
    minWidth: 32,

    "&:hover": {
      background: theme.palette.background.paper,
    },
  },
  fileCopyIcon: {
    width: 20,
    height: 20,
  },
  buttonCopy: {
    marginLeft: theme.spacing(1),
  },
}))
