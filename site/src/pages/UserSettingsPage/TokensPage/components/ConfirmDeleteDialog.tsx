import { FC } from "react"
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import { useTranslation, Trans } from "react-i18next"
import { useDeleteToken } from "../hooks"
import { displaySuccess, displayError } from "components/GlobalSnackbar/utils"
import { getErrorMessage } from "api/errors"

export const ConfirmDeleteDialog: FC<{
  queryKey: (string | boolean)[]
  tokenId: string | undefined
  setTokenId: (arg: string | undefined) => void
}> = ({ queryKey, tokenId, setTokenId }) => {
  const { t } = useTranslation("tokensPage")

  const description = (
    <Trans t={t} i18nKey="deleteToken.deleteCaption" values={{ tokenId }}>
      Are you sure you want to delete this token?
      <br />
      <br />
      {{ tokenId }}
    </Trans>
  )

  const { mutate: deleteToken, isLoading: isDeleting } =
    useDeleteToken(queryKey)

  const onDeleteSuccess = () => {
    displaySuccess(t("deleteToken.deleteSuccess"))
    setTokenId(undefined)
  }

  const onDeleteError = (error: unknown) => {
    const message = getErrorMessage(error, t("deleteToken.deleteFailure"))
    displayError(message)
    setTokenId(undefined)
  }

  return (
    <ConfirmDialog
      title={t("deleteToken.delete")}
      description={description}
      open={Boolean(tokenId) || isDeleting}
      confirmLoading={isDeleting}
      onConfirm={() => {
        if (!tokenId) {
          return
        }
        deleteToken(tokenId, {
          onError: onDeleteError,
          onSuccess: onDeleteSuccess,
        })
      }}
      onClose={() => {
        setTokenId(undefined)
      }}
    />
  )
}
