import IconButton from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import Tooltip from "@material-ui/core/Tooltip"
import Check from "@material-ui/icons/Check"
import React, { useState } from "react"
import { combineClasses } from "../../util/combineClasses"
import { FileCopyIcon } from "../Icons/FileCopyIcon"

interface CopyButtonProps {
  text: string
  ctaCopy?: string
  wrapperClassName?: string
  buttonClassName?: string
}

/**
 * Copy button used inside the CodeBlock component internally
 */
export const CopyButton: React.FC<React.PropsWithChildren<CopyButtonProps>> = ({
  text,
  ctaCopy,
  wrapperClassName = "",
  buttonClassName = "",
}) => {
  const styles = useStyles()
  const [isCopied, setIsCopied] = useState<boolean>(false)

  const copyToClipboard = async (): Promise<void> => {
    try {
      await window.navigator.clipboard.writeText(text)
      setIsCopied(true)
      window.setTimeout(() => {
        setIsCopied(false)
      }, 1000)
    } catch (err) {
      const input = document.createElement("input")
      input.value = text
      document.body.appendChild(input)
      input.focus()
      input.select()
      const result = document.execCommand("copy")
      document.body.removeChild(input)
      if (result) {
        setIsCopied(true)
        window.setTimeout(() => {
          setIsCopied(false)
        }, 1000)
      } else {
        const wrappedErr = new Error("copyToClipboard: failed to copy text to clipboard")
        if (err instanceof Error) {
          wrappedErr.stack = err.stack
        }
        console.error(wrappedErr)
      }
    }
  }

  return (
    <Tooltip title="Copy to Clipboard" placement="top">
      <div className={combineClasses([styles.copyButtonWrapper, wrapperClassName])}>
        <IconButton
          className={combineClasses([styles.copyButton, buttonClassName])}
          onClick={copyToClipboard}
          size="small"
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
    marginLeft: theme.spacing(1),
  },
  copyButton: {
    borderRadius: 7,
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
