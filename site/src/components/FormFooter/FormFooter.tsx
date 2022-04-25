import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import React from "react"
import { LoadingButton } from "../LoadingButton/LoadingButton"

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

export const FormFooter: React.FC<FormFooterProps> = ({ onCancel, isLoading, submitLabel = "Submit" }) => {
  const styles = useStyles()
  return (
    <div className={styles.footer}>
      <Button className={styles.button} onClick={onCancel} variant="outlined">
        Cancel
      </Button>
      <LoadingButton loading={isLoading} className={styles.button} variant="contained" color="primary" type="submit">
        {submitLabel}
      </LoadingButton>
    </div>
  )
}
