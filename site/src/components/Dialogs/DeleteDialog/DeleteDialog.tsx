import makeStyles from "@mui/styles/makeStyles";
import TextField from "@mui/material/TextField";
import { Maybe } from "components/Conditionals/Maybe";
import { ChangeEvent, useState, PropsWithChildren, FC } from "react";
import { useTranslation } from "react-i18next";
import { ConfirmDialog } from "../ConfirmDialog/ConfirmDialog";

export interface DeleteDialogProps {
  isOpen: boolean;
  onConfirm: () => void;
  onCancel: () => void;
  entity: string;
  name: string;
  info?: string;
  confirmLoading?: boolean;
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
  const styles = useStyles();
  const { t } = useTranslation("common");
  const [nameValue, setNameValue] = useState("");
  const confirmed = name === nameValue;
  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    setNameValue(event.target.value);
  };
  const hasError = nameValue.length > 0 && !confirmed;

  const content = (
    <>
      <p>{t("deleteDialog.intro", { entity })}</p>
      <Maybe condition={info !== undefined}>
        <p className={styles.warning}>{info}</p>
      </Maybe>
      <p>{t("deleteDialog.confirm", { entity, name })}</p>

      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (confirmed) {
            onConfirm();
          }
        }}
      >
        <TextField
          fullWidth
          autoFocus
          className={styles.textField}
          name="confirmation"
          autoComplete="off"
          id="confirmation"
          placeholder={name}
          value={nameValue}
          onChange={handleChange}
          label={t("deleteDialog.confirmLabel", { entity })}
          error={hasError}
          helperText={hasError && t("deleteDialog.incorrectName", { entity })}
        />
      </form>
    </>
  );

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
  );
};

const useStyles = makeStyles((theme) => ({
  warning: {
    color: theme.palette.warning.light,
  },

  textField: {
    marginTop: theme.spacing(3),
  },
}));
