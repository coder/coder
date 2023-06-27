import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import { useTranslation } from "react-i18next"

export const InactivityDialog = ({
  submitValues,
  isInactivityDialogOpen,
  setIsInactivityDialogOpen,
  numberWorkspacesToBeDeletedToday,
}: {
  submitValues: () => void
  isInactivityDialogOpen: boolean
  setIsInactivityDialogOpen: (arg0: boolean) => void
  numberWorkspacesToBeDeletedToday: number
}) => {
  const { t } = useTranslation("templateSettingsPage")

  return (
    <ConfirmDialog
      type="delete"
      open={isInactivityDialogOpen}
      onConfirm={() => {
        submitValues()
        setIsInactivityDialogOpen(false)
      }}
      onClose={() => setIsInactivityDialogOpen(false)}
      title="Delete inactive workspaces"
      confirmText="Delete Workspaces"
      description={t("inactivityDialogDescription", { count: numberWorkspacesToBeDeletedToday })}
    />
  )
}
