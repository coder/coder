import { FC, PropsWithChildren, useState } from "react"
import { Section } from "components/SettingsLayout/Section"
import { TokensPageView } from "./TokensPageView"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { useTranslation, Trans } from "react-i18next"
import { useTokensData } from "./hooks"
import { ConfirmDeleteDialog } from "./components"
import { Stack } from "components/Stack/Stack"
import Button from "@material-ui/core/Button"
import { Link as RouterLink } from "react-router-dom"
import AddIcon from "@material-ui/icons/AddOutlined"
import { APIKeyWithOwner } from "api/typesGenerated"

export const TokensPage: FC<PropsWithChildren<unknown>> = () => {
  const styles = useStyles()
  const { t } = useTranslation("tokensPage")

  const cliCreateCommand = "coder tokens create"
  const description = (
    <Trans t={t} i18nKey="description" values={{ cliCreateCommand }}>
      Tokens are used to authenticate with the Coder API. You can create a token
      with the Coder CLI using the <code>{{ cliCreateCommand }}</code> command.
    </Trans>
  )

  const TokenActions = () => (
    <Stack direction="row" justifyContent="end" className={styles.tokenActions}>
      <Button startIcon={<AddIcon />} component={RouterLink} to="new">
        {t("tokenActions.addToken")}
      </Button>
    </Stack>
  )

  const [tokenToDelete, setTokenToDelete] = useState<
    APIKeyWithOwner | undefined
  >(undefined)

  const {
    data: tokens,
    error: getTokensError,
    isFetching,
    isFetched,
    queryKey,
  } = useTokensData({
    // we currently do not show all tokens in the UI, even if
    // the user has read all permissions
    include_all: false,
  })

  return (
    <>
      <Section
        title={t("title")}
        className={styles.section}
        description={description}
        layout="fluid"
      >
        <TokenActions />
        <TokensPageView
          tokens={tokens}
          isLoading={isFetching}
          hasLoaded={isFetched}
          getTokensError={getTokensError}
          onDelete={(token) => {
            setTokenToDelete(token)
          }}
        />
      </Section>
      <ConfirmDeleteDialog
        queryKey={queryKey}
        token={tokenToDelete}
        setToken={setTokenToDelete}
      />
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  section: {
    "& code": {
      background: theme.palette.divider,
      fontSize: 12,
      padding: "2px 4px",
      color: theme.palette.text.primary,
      borderRadius: 2,
    },
  },
  tokenActions: {
    marginBottom: theme.spacing(1),
  },
}))

export default TokensPage
