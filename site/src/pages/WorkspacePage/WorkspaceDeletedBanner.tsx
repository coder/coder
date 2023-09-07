import Button from "@mui/material/Button";
import { FC } from "react";
import { Alert } from "components/Alert/Alert";
import { useTranslation } from "react-i18next";

export interface WorkspaceDeletedBannerProps {
  handleClick: () => void;
}

export const WorkspaceDeletedBanner: FC<
  React.PropsWithChildren<WorkspaceDeletedBannerProps>
> = ({ handleClick }) => {
  const { t } = useTranslation("workspacePage");

  const NewWorkspaceButton = (
    <Button onClick={handleClick} size="small" variant="text">
      {t("ctas.createWorkspaceCta")}
    </Button>
  );

  return (
    <Alert severity="warning" actions={NewWorkspaceButton}>
      {t("warningsAndErrors.workspaceDeletedWarning")}
    </Alert>
  );
};
