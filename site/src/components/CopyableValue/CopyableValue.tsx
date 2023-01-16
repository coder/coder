import { makeStyles } from "@material-ui/core/styles"
import Tooltip from "@material-ui/core/Tooltip"
import { useClickable } from "hooks/useClickable"
import { useClipboard } from "hooks/useClipboard"
import { FC, HTMLProps } from "react"
import { combineClasses } from "util/combineClasses"

interface CopyableValueProps extends HTMLProps<HTMLDivElement> {
  value: string
}

export const CopyableValue: FC<CopyableValueProps> = ({
  value,
  className,
  ...props
}) => {
  const { isCopied, copy } = useClipboard(value)
  const clickableProps = useClickable(copy)
  const styles = useStyles()

  return (
    <Tooltip
      title={isCopied ? "Copied!" : "Click to copy"}
      placement="bottom-start"
    >
      <span
        {...props}
        {...clickableProps}
        className={combineClasses([styles.value, className])}
      />
    </Tooltip>
  )
}

const useStyles = makeStyles(() => ({
  value: {
    cursor: "pointer",
  },
}))
