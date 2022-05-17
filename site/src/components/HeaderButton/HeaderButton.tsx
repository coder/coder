import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import React from "react"

export interface HeaderButtonProps {
  readonly text: string
  readonly disabled?: boolean
  readonly onClick?: (event: MouseEvent) => void
}

export const HeaderButton: React.FC<HeaderButtonProps> = (props) => {
  const styles = useStyles()

  return (
    <Button
      className={styles.pageButton}
      variant="contained"
      onClick={(event: React.MouseEvent): void => {
        if (props.onClick) {
          props.onClick(event.nativeEvent)
        }
      }}
      disabled={props.disabled}
      component="button"
    >
      {props.text}
    </Button>
  )
}

const useStyles = makeStyles(() => ({
  pageButton: {
    whiteSpace: "nowrap",
  },
}))
