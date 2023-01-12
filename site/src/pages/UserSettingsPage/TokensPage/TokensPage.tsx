import { FC, PropsWithChildren } from "react"
import { Section } from "../../../components/Section/Section"
import { TokensPageView } from "./TokensPageView"
import { tokensMachine } from "xServices/tokens/tokensXService"
import { useMachine } from "@xstate/react"
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import { Typography } from "components/Typography/Typography"

export const Language = {
  title: "Tokens",
  description: (
    <p>
      Tokens are used to authenticate with the Coder API and can be created with
      the Coder CLI.
    </p>
  ),
  deleteTitle: "Delete Token",
  deleteDescription: "Are you sure you want to delete this token?",
}

export const TokensPage: FC<PropsWithChildren<unknown>> = () => {
  const [tokensState, tokensSend] = useMachine(tokensMachine)
  const isLoading = tokensState.matches("gettingTokens")
  const hasLoaded = tokensState.matches("loaded")
  const { getTokensError, tokens, deleteTokenId } = tokensState.context
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
      <Section
        title={Language.title}
        description={Language.description}
        layout="fluid"
      >
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

export default TokensPage
