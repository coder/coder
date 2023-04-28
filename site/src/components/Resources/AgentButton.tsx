import { makeStyles } from "@material-ui/core/styles"
import Button, { ButtonProps } from "@material-ui/core/Button"
import { FC, forwardRef } from "react"
import { combineClasses } from "utils/combineClasses"

export const PrimaryAgentButton: FC<ButtonProps> = ({
  className,
  ...props
}) => {
  const styles = useStyles()

  return (
    <Button
      className={combineClasses([styles.primaryButton, className])}
      {...props}
    />
  )
}

// eslint-disable-next-line react/display-name -- Name is inferred from variable name
export const SecondaryAgentButton = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, ...props }, ref) => {
    const styles = useStyles()

    return (
      <Button
        ref={ref}
        variant="outlined"
        className={combineClasses([styles.secondaryButton, className])}
        {...props}
      />
    )
  },
)

const useStyles = makeStyles((theme) => ({
  primaryButton: {
    whiteSpace: "nowrap",
    backgroundColor: theme.palette.background.default,
    height: 36,
    minHeight: 36,
    borderRadius: 4,
    fontWeight: 500,
    fontSize: 14,

    "&:hover": {
      backgroundColor: `${theme.palette.background.paper} !important`,
    },

    "& .MuiButton-startIcon": {
      width: 12,
      height: 12,
      marginRight: theme.spacing(1.5),

      "& svg": {
        width: "100%",
        height: "100%",
      },
    },
  },

  secondaryButton: {
    fontSize: 14,
    fontWeight: 500,
    height: 36,
    minHeight: 36,
    borderRadius: 4,
  },
}))
