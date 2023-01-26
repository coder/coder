import FormHelperText from "@material-ui/core/FormHelperText"
import makeStyles from "@material-ui/core/styles/makeStyles"
import TextField from "@material-ui/core/TextField"
import Typography from "@material-ui/core/Typography"
import { Maybe } from "components/Conditionals/Maybe"
import { Stack } from "components/Stack/Stack"
import { ChangeEvent, useState, PropsWithChildren, FC } from "react"
import { useTranslation } from "react-i18next"
import { ConfirmDialog } from "../ConfirmDialog/ConfirmDialog"

export interface DeleteDialogProps {
  isOpen: boolean
  onConfirm: () => void
  onCancel: () => void
  entity: string
  name: string
  info?: string
  confirmLoading?: boolean
}

export const DeleteDialog: FC<PropsWithChildren<DeleteDialogProps>> = ({
  isOpen,
  onCancel,
  onConfirm,
  entity,
  info,
  name,
  confirmLoading,
}) => {
  const styles = useStyles()
  const { t } = useTranslation("common")
  const [nameValue, setNameValue] = useState("")
  const confirmed = name === nameValue
  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    setNameValue(event.target.value)
  }

  const content = (
    <>
      <Typography>{t("deleteDialog.intro", { entity })}</Typography>
      <Maybe condition={info !== undefined}>
        <Typography className={styles.warning}>{info}</Typography>
      </Maybe>
      <Typography>{t("deleteDialog.confirm", { entity })}</Typography>
      <Stack spacing={1}>
        <TextField
          name="confirmation"
          autoComplete="off"
          id="confirmation"
          placeholder={name}
          value={nameValue}
          onChange={handleChange}
          label={t("deleteDialog.confirmLabel", { entity })}
        />
        <Maybe condition={nameValue.length > 0 && !confirmed}>
          <FormHelperText error>
            {t("deleteDialog.incorrectName", { entity })}
          </FormHelperText>
        </Maybe>
      </Stack>
    </>
  )

  return (
    <ConfirmDialog
      type="delete"
      hideCancel={false}
      open={isOpen}
      title={t("deleteDialog.title", { entity })}
      onConfirm={onConfirm}
      onClose={onCancel}
      description={content}
      confirmLoading={confirmLoading}
      disabled={!confirmed}
    />
  )
}

const useStyles = makeStyles((theme) => ({
  warning: {
    color: theme.palette.warning.light,
  },
}))
