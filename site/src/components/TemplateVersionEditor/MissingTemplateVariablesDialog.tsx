import { makeStyles } from "@mui/styles"
import Dialog from "@mui/material/Dialog"
import DialogContent from "@mui/material/DialogContent"
import DialogContentText from "@mui/material/DialogContentText"
import DialogTitle from "@mui/material/DialogTitle"
import { DialogProps } from "components/Dialogs/Dialog"
import { FC, useEffect, useState } from "react"
import { FormFields, VerticalForm } from "components/Form/Form"
import { TemplateVersionVariable, VariableValue } from "api/typesGenerated"
import DialogActions from "@mui/material/DialogActions"
import Button from "@mui/material/Button"
import { VariableInput } from "pages/CreateTemplatePage/VariableInput"
import { Loader } from "components/Loader/Loader"

export type MissingTemplateVariablesDialogProps = Omit<
  DialogProps,
  "onSubmit"
> & {
  onClose: () => void
  onSubmit: (values: VariableValue[]) => void
  missingVariables?: TemplateVersionVariable[]
}

export const MissingTemplateVariablesDialog: FC<
  MissingTemplateVariablesDialogProps
> = ({ missingVariables, onSubmit, ...dialogProps }) => {
  const styles = useStyles()
  const [variableValues, setVariableValues] = useState<VariableValue[]>([])

  // Pre-fill the form with the default values when missing variables are loaded
  useEffect(() => {
    if (!missingVariables) {
      return
    }
    setVariableValues(
      missingVariables.map((v) => ({ name: v.name, value: v.value })),
    )
  }, [missingVariables])

  return (
    <Dialog
      {...dialogProps}
      scroll="body"
      aria-labelledby="update-build-parameters-title"
      maxWidth="xs"
      data-testid="dialog"
    >
      <DialogTitle
        id="update-build-parameters-title"
        classes={{ root: styles.title }}
      >
        Template variables
      </DialogTitle>
      <DialogContent className={styles.content}>
        <DialogContentText className={styles.info}>
          There are a few missing template variable values. Please fill them in.
        </DialogContentText>
        <VerticalForm
          className={styles.form}
          id="updateVariables"
          onSubmit={(e) => {
            e.preventDefault()
            onSubmit(variableValues)
          }}
        >
          {missingVariables ? (
            <FormFields>
              {missingVariables.map((variable, index) => {
                return (
                  <VariableInput
                    defaultValue={variable.value}
                    variable={variable}
                    key={variable.name}
                    onChange={async (value) => {
                      setVariableValues((prev) => {
                        prev[index] = {
                          name: variable.name,
                          value,
                        }
                        return [...prev]
                      })
                    }}
                  />
                )
              })}
            </FormFields>
          ) : (
            <Loader />
          )}
        </VerticalForm>
      </DialogContent>
      <DialogActions disableSpacing className={styles.dialogActions}>
        <Button color="primary" fullWidth type="submit" form="updateVariables">
          Submit
        </Button>
        <Button fullWidth type="button" onClick={dialogProps.onClose}>
          Cancel
        </Button>
      </DialogActions>
    </Dialog>
  )
}

const useStyles = makeStyles((theme) => ({
  title: {
    padding: theme.spacing(3, 5),

    "& h2": {
      fontSize: theme.spacing(2.5),
      fontWeight: 400,
    },
  },

  content: {
    padding: theme.spacing(0, 5, 0, 5),
  },

  info: {
    margin: 0,
  },

  form: {
    paddingTop: theme.spacing(4),
  },

  infoTitle: {
    fontSize: theme.spacing(2),
    fontWeight: 600,
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(1),
  },

  formFooter: {
    flexDirection: "column",
  },

  dialogActions: {
    padding: theme.spacing(5),
    flexDirection: "column",
    gap: theme.spacing(1),
  },
}))
