import { makeStyles } from "@material-ui/core/styles"
import { FC } from "react"
import { ButtonMapping, ButtonTypesEnum } from "../constants"

export interface DropdownContentProps {
  secondaryActions: ButtonTypesEnum[]
  buttonMapping: Partial<ButtonMapping>
}

/* secondary workspace CTAs */
export const DropdownContent: FC<React.PropsWithChildren<DropdownContentProps>> = ({
  secondaryActions,
  buttonMapping,
}) => {
  const styles = useStyles()

  return (
    <span data-testid="secondary-ctas">
      {secondaryActions.map((action) => (
        <div key={action} className={styles.popoverActionButton}>
          {buttonMapping[action]}
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
