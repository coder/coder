import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import { LoadingButton } from "../LoadingButton/LoadingButton"

export const Language = {
  cancelLabel: "Cancel",
  defaultSubmitLabel: "Submit",
}

export interface FormFooterProps {
  onCancel: () => void
  isLoading: boolean
  submitLabel?: string
}

const useStyles = makeStyles(() => ({
  footer: {
    display: "flex",
    flex: "0",
    flexDirection: "row",
    justifyContent: "center",
    alignItems: "center",
  },
  button: {
    margin: "1em",
  },
}))

export const FormFooter: React.FC<FormFooterProps> = ({
  onCancel,
  isLoading,
  submitLabel = Language.defaultSubmitLabel,
}) => {
  const styles = useStyles()
  return (
    <div className={styles.footer}>
      <Button type="button" className={styles.button} onClick={onCancel} variant="outlined">
        {Language.cancelLabel}
      </Button>
      <LoadingButton loading={isLoading} className={styles.button} variant="contained" color="primary" type="submit">
        {submitLabel}
      </LoadingButton>
    </div>
  )
}
