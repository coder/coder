import { FC, PropsWithChildren, useState } from "react"
import { Section } from "components/SettingsLayout/Section"
import { TokensPageView } from "./TokensPageView"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { useTranslation, Trans } from "react-i18next"
import { useTokensData, useCheckTokenPermissions } from "./hooks"
import { TokensSwitch, ConfirmDeleteDialog } from "./components"

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

  const [tokenIdToDelete, setTokenIdToDelete] = useState<string | undefined>(
    undefined,
  )
  const [viewAllTokens, setViewAllTokens] = useState<boolean>(false)
  const { data: perms } = useCheckTokenPermissions()

  const {
    data: tokens,
    error: getTokensError,
    isFetching,
    isFetched,
    queryKey,
  } = useTokensData({
    include_all: viewAllTokens,
  })

  return (
    <>
      <Section
        title={t("title")}
        className={styles.section}
        description={description}
        layout="fluid"
      >
        <TokensSwitch
          hasReadAll={perms?.readAllApiKeys ?? false}
          viewAllTokens={viewAllTokens}
          setViewAllTokens={setViewAllTokens}
        />
        <TokensPageView
          tokens={tokens}
          viewAllTokens={viewAllTokens}
          isLoading={isFetching}
          hasLoaded={isFetched}
          getTokensError={getTokensError}
          onDelete={(id) => {
            setTokenIdToDelete(id)
          }}
        />
      </Section>
      <ConfirmDeleteDialog
        queryKey={queryKey}
        tokenId={tokenIdToDelete}
        setTokenId={setTokenIdToDelete}
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
}))

export default TokensPage
