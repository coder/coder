import Button from "@mui/material/Button";
import { FC } from "react";
import { Alert } from "components/Alert/Alert";

export interface WorkspaceDeletedBannerProps {
  handleClick: () => void;
}

export const WorkspaceDeletedBanner: FC<
  React.PropsWithChildren<WorkspaceDeletedBannerProps>
> = ({ handleClick }) => {
  const NewWorkspaceButton = (
    <Button onClick={handleClick} size="small" variant="text">
      Create new workspace
    </Button>
  );

  return (
    <Alert severity="warning" actions={NewWorkspaceButton}>
      This workspace has been deleted and cannot be edited.
    </Alert>
  );
};
