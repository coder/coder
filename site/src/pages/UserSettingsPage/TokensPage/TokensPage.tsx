import { FC, PropsWithChildren } from "react"
import { Section } from "../../../components/Section/Section"
import { TokensPageView } from "./TokensPageView"
import { tokensMachine } from "xServices/tokens/tokensXService"
import { useMachine } from "@xstate/react"
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog"

export const Language = {
  title: "Tokens",
  description: (
    <p>
      Tokens are used to authenticate with the Coder API and can be created with the Coder CLI.
    </p>
  ),
}

export const TokensPage: FC<PropsWithChildren<unknown>> = () => {
  const [tokensState, tokensSend] = useMachine(tokensMachine)
  const isLoading = tokensState.matches("gettingTokens")
  const hasLoaded = tokensState.matches("loaded")
  const { getTokensError, tokens, deleteTokenId } = tokensState.context

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

      <DeleteDialog
          isOpen={tokensState.matches("confirmTokenDelete")}
          confirmLoading={tokensState.matches("deletingToken")}
          name={deleteTokenId ? deleteTokenId : ""}
          entity="token"
          onConfirm={() => {
            tokensSend("CONFIRM_DELETE_TOKEN")
          }}
          onCancel={() => {
            tokensSend("CANCEL_DELETE_TOKEN")
          }}
        />
    </>
  )
}

export default TokensPage
