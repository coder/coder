import makeStyles from "@mui/styles/makeStyles";
import TextField from "@mui/material/TextField";
import { ChangeEvent, useState, PropsWithChildren, FC } from "react";
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
  const [nameValue, setNameValue] = useState("");
  const confirmed = name === nameValue;
  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    setNameValue(event.target.value);
  };
  const hasError = nameValue.length > 0 && !confirmed;

  const content = (
    <>
      <p>Deleting this {entity} is irreversible!</p>
      {Boolean(info) && <p className={styles.warning}>{info}</p>}
      <p>Are you sure you want to proceed?</p>
      <p>
        Type &ldquo;<strong>{name}</strong>&rdquo; below to confirm.
      </p>

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
          label={`Name of the ${entity} to delete`}
          error={hasError}
          helperText={
            hasError && `${nameValue} does not match the name of this ${entity}`
          }
          inputProps={{ ["data-testid"]: "delete-dialog-name-confirmation" }}
        />
      </form>
    </>
  );

  return (
    <ConfirmDialog
      type="delete"
      hideCancel={false}
      open={isOpen}
      title={`Delete ${entity}`}
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
