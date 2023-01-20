import { FC, PropsWithChildren } from "react"
import { Section } from "../../../components/SettingsLayout/Section"
import { TokensPageView } from "./TokensPageView"
import { tokensMachine } from "xServices/tokens/tokensXService"
import { useMachine } from "@xstate/react"
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import { Typography } from "components/Typography/Typography"
import makeStyles from "@material-ui/core/styles/makeStyles"

export const Language = {
  title: "Tokens",
  descriptionPrefix:
    "Tokens are used to authenticate with the Coder API. You can create a token with the Coder CLI using the ",
  deleteTitle: "Delete Token",
  deleteDescription: "Are you sure you want to delete this token?",
}

export const TokensPage: FC<PropsWithChildren<unknown>> = () => {
  const [tokensState, tokensSend] = useMachine(tokensMachine)
  const isLoading = tokensState.matches("gettingTokens")
  const hasLoaded = tokensState.matches("loaded")
  const { getTokensError, tokens, deleteTokenId } = tokensState.context
  const styles = useStyles()
  const description = (
    <>
      {Language.descriptionPrefix}{" "}
      <code className={styles.code}>coder tokens create</code> command.
    </>
  )

  const content = (
    <Typography>
      {Language.deleteDescription}
      <br />
      <br />
      {deleteTokenId}
    </Typography>
  )

  return (
    <>
      <Section title={Language.title} description={description} layout="fluid">
        <TokensPageView
          tokens={tokens}
          isLoading={isLoading}
          hasLoaded={hasLoaded}
          getTokensError={getTokensError}
          onDelete={(id) => {
            tokensSend({ type: "DELETE_TOKEN", id })
          }}
        />
      </Section>

      <ConfirmDialog
        title={Language.deleteTitle}
        description={content}
        open={tokensState.matches("confirmTokenDelete")}
        confirmLoading={tokensState.matches("deletingToken")}
        onConfirm={() => {
          tokensSend("CONFIRM_DELETE_TOKEN")
        }}
        onClose={() => {
          tokensSend("CANCEL_DELETE_TOKEN")
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
}))

export default TokensPage
