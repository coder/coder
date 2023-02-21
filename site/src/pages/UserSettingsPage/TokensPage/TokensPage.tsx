import { FC, PropsWithChildren, useState } from "react"
import { Section } from "../../../components/SettingsLayout/Section"
import { TokensPageView } from "./TokensPageView"
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import { Typography } from "components/Typography/Typography"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { useTranslation } from "react-i18next"
import { useTokensData, useDeleteToken } from "./hooks"
import { displaySuccess, displayError } from "components/GlobalSnackbar/utils"
import { getErrorMessage } from "api/errors"

export const TokensPage: FC<PropsWithChildren<unknown>> = () => {
  const styles = useStyles()
  const { t } = useTranslation("tokensPage")
  const [tokenIdToDelete, setTokenIdToDelete] = useState<string | undefined>(
    undefined,
  )

  const {
    data: tokens,
    error: getTokensError,
    isFetching,
    isFetched,
    queryKey,
  } = useTokensData({
    include_all: true,
  })

  const { mutate: deleteToken, isLoading: isDeleting } =
    useDeleteToken(queryKey)

  const onDeleteSuccess = () => {
    displaySuccess(t("deleteToken.deleteSuccess"))
    setTokenIdToDelete(undefined)
  }

  const onDeleteError = (error: unknown) => {
    const message = getErrorMessage(error, t("deleteToken.deleteFailure"))
    displayError(message)
    setTokenIdToDelete(undefined)
  }

  const description = (
    <>
      {t("description")}{" "}
      <code className={styles.code}>coder tokens create</code> command.
    </>
  )

  const content = (
    <Typography>
      {t("deleteToken.deleteCaption")}
      <br />
      <br />
      {tokenIdToDelete}
    </Typography>
  )

  return (
    <>
      <Section title={t("title")} description={description} layout="fluid">
        <TokensPageView
          tokens={tokens}
          isLoading={isFetching}
          hasLoaded={isFetched}
          getTokensError={getTokensError}
          onDelete={(id) => {
            setTokenIdToDelete(id)
          }}
        />
      </Section>

      <ConfirmDialog
        title={t("deleteToken.delete")}
        description={content}
        open={Boolean(tokenIdToDelete) || isDeleting}
        confirmLoading={isDeleting}
        onConfirm={() => {
          if (!tokenIdToDelete) {
            return
          }
          deleteToken(tokenIdToDelete, {
            onError: onDeleteError,
            onSuccess: onDeleteSuccess,
          })
        }}
        onClose={() => {
          setTokenIdToDelete(undefined)
        }}
      />
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  code: {
    background: theme.palette.divider,
    fontSize: 12,
    padding: "2px 4px",
    color: theme.palette.text.primary,
    borderRadius: 2,
  },
  formRow: {
    justifyContent: "end",
    marginBottom: "10px",
  },
}))

export default TokensPage
