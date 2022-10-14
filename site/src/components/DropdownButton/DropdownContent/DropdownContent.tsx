import { makeStyles } from "@material-ui/core/styles"
import { FC, ReactNode } from "react"

export interface DropdownContentProps {
  secondaryActions: Array<{ action: string; button: ReactNode }>
}

/* secondary workspace CTAs */
export const DropdownContent: FC<
  React.PropsWithChildren<DropdownContentProps>
> = ({ secondaryActions }) => {
  const styles = useStyles()

  return (
    <span data-testid="secondary-ctas">
      {secondaryActions.map(({ action, button }) => (
        <div key={action} className={styles.popoverActionButton}>
          {button}
        </div>
      ))}
    </span>
  )
}

const useStyles = makeStyles(() => ({
  popoverActionButton: {
    "& .MuiButtonBase-root": {
      backgroundColor: "unset",
      justifyContent: "start",
      padding: "0px",
    },
  },
}))
