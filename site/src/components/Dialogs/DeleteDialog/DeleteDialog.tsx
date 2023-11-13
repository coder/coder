import {
  type FC,
  type FormEvent,
  type PropsWithChildren,
  useId,
  useState,
} from "react";
import TextField from "@mui/material/TextField";
import { ConfirmDialog } from "../ConfirmDialog/ConfirmDialog";
import { Callout } from "../../Callout/Callout";

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
  const hookId = useId();

  const [userConfirmationText, setUserConfirmationText] = useState("");
  const [isFocused, setIsFocused] = useState(false);

  const deletionConfirmed = name === userConfirmationText;
  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (deletionConfirmed) {
      onConfirm();
    }
  };

  const hasError = !deletionConfirmed && userConfirmationText.length > 0;
  const displayErrorMessage = hasError && !isFocused;
  const inputColor = hasError ? "error" : "primary";

  return (
    <ConfirmDialog
      type="danger"
      hideCancel={false}
      open={isOpen}
      title={`Delete ${entity}`}
      onConfirm={onConfirm}
      onClose={onCancel}
      confirmLoading={confirmLoading}
      disabled={!deletionConfirmed}
      description={
        <>
          <p>Deleting this {entity} is irreversible!</p>

          {Boolean(info) && <Callout type="danger">{info}</Callout>}

          <p>Are you sure you want to proceed?</p>

          <p>
            Type <strong>{name}</strong> below to confirm.
          </p>

          <form onSubmit={onSubmit}>
            <TextField
              fullWidth
              autoFocus
              css={{ marginTop: 24 }}
              name="confirmation"
              autoComplete="off"
              id={`${hookId}-confirm`}
              placeholder={name}
              value={userConfirmationText}
              onChange={(event) => setUserConfirmationText(event.target.value)}
              onFocus={() => setIsFocused(true)}
              onBlur={() => setIsFocused(false)}
              label={`Name of the ${entity} to delete`}
              color={inputColor}
              error={displayErrorMessage}
              helperText={
                displayErrorMessage &&
                `${userConfirmationText} does not match the name of this ${entity}`
              }
              InputProps={{ color: inputColor }}
              inputProps={{
                "data-testid": "delete-dialog-name-confirmation",
              }}
            />
          </form>
        </>
      }
    />
  );
};
