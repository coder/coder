import { makeStyles } from "@mui/styles";
import { FC } from "react";
import * as TypesGen from "api/typesGenerated";
import { CodeExample } from "components/CodeExample/CodeExample";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";

export interface ResetPasswordDialogProps {
  open: boolean;
  onClose: () => void;
  onConfirm: () => void;
  user?: TypesGen.User;
  newPassword?: string;
  loading: boolean;
}

export const Language = {
  title: "Reset password",
  message: (username?: string): JSX.Element => (
    <>
      You will need to send <strong>{username}</strong> the following password:
    </>
  ),
  confirmText: "Reset password",
};

export const ResetPasswordDialog: FC<
  React.PropsWithChildren<ResetPasswordDialogProps>
> = ({ open, onClose, onConfirm, user, newPassword, loading }) => {
  const styles = useStyles();

  const description = (
    <>
      <p>{Language.message(user?.username)}</p>
      <CodeExample code={newPassword ?? ""} className={styles.codeExample} />
    </>
  );

  return (
    <ConfirmDialog
      type="info"
      hideCancel={false}
      open={open}
      onConfirm={onConfirm}
      onClose={onClose}
      title={Language.title}
      confirmLoading={loading}
      confirmText={Language.confirmText}
      description={description}
    />
  );
};

const useStyles = makeStyles((theme) => ({
  codeExample: {
    minHeight: "auto",
    userSelect: "all",
    width: "100%",
    marginTop: theme.spacing(3),
  },
}));
