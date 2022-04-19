import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import Tooltip from "@material-ui/core/Tooltip"
import Check from "@material-ui/icons/Check"
import React, { useState } from "react"
import { FileCopyIcon } from "../Icons/FileCopyIcon"

interface CopyButtonProps {
  text: string
  className?: string
}

/**
 * Copy button used inside the CodeBlock component internally
 */
export const CopyButton: React.FC<CopyButtonProps> = ({ className = "", text }) => {
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
      const wrappedErr = new Error("copyToClipboard: failed to copy text to clipboard")
      if (err instanceof Error) {
        wrappedErr.stack = err.stack
      }
      console.error(wrappedErr)
    }
  }

  return (
    <Tooltip title="Copy to Clipboard" placement="top">
      <div className={`${styles.copyButtonWrapper} ${className}`}>
        <Button className={styles.copyButton} onClick={copyToClipboard} size="small">
          {isCopied ? <Check className={styles.fileCopyIcon} /> : <FileCopyIcon className={styles.fileCopyIcon} />}
        </Button>
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
    background: theme.palette.codeBlock.button.main,
    color: theme.palette.codeBlock.button.contrastText,
    padding: theme.spacing(0.85),
    minWidth: 32,

    "&:hover": {
      background: theme.palette.codeBlock.button.hover,
    },
  },
  fileCopyIcon: {
    width: 20,
    height: 20,
  },
}))
