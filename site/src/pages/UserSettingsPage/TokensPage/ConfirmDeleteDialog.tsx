import { FC } from "react"
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import { useTranslation, Trans } from "react-i18next"
import { useDeleteToken } from "./hooks"
import { displaySuccess, displayError } from "components/GlobalSnackbar/utils"
import { getErrorMessage } from "api/errors"
import { APIKeyWithOwner } from "api/typesGenerated"

export interface ConfirmDeleteDialogProps {
  queryKey: (string | boolean)[]
  token: APIKeyWithOwner | undefined
  setToken: (arg: APIKeyWithOwner | undefined) => void
}

export const ConfirmDeleteDialog: FC<ConfirmDeleteDialogProps> = ({
  queryKey,
  token,
  setToken,
}) => {
  const { t } = useTranslation("tokensPage")
  const tokenName = token?.token_name
  const description = (
    <Trans
      t={t}
      i18nKey="tokenActions.deleteToken.deleteCaption"
      values={{ tokenName }}
    >
      Are you sure you want to permanently delete token {{ tokenName }}?
    </Trans>
  )

  const { mutate: deleteToken, isLoading: isDeleting } =
    useDeleteToken(queryKey)

  const onDeleteSuccess = () => {
    displaySuccess(t("tokenActions.deleteToken.deleteSuccess"))
    setToken(undefined)
  }

  const onDeleteError = (error: unknown) => {
    const message = getErrorMessage(
      error,
      t("tokenActions.deleteToken.deleteFailure"),
    )
    displayError(message)
    setToken(undefined)
  }

  return (
    <ConfirmDialog
      type="delete"
      title={t("tokenActions.deleteToken.delete")}
      description={description}
      open={Boolean(token) || isDeleting}
      confirmLoading={isDeleting}
      onConfirm={() => {
        if (!token) {
          return
        }
        deleteToken(token.id, {
          onError: onDeleteError,
          onSuccess: onDeleteSuccess,
        })
      }}
      onClose={() => {
        setToken(undefined)
      }}
    />
  )
}
