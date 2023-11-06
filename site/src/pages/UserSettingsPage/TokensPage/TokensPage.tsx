import { FC, PropsWithChildren, useState } from "react";
import { Section } from "components/SettingsLayout/Section";
import { TokensPageView } from "./TokensPageView";
import { useTokensData } from "./hooks";
import { ConfirmDeleteDialog } from "./ConfirmDeleteDialog";
import { Stack } from "components/Stack/Stack";
import Button from "@mui/material/Button";
import { Link as RouterLink } from "react-router-dom";
import AddIcon from "@mui/icons-material/AddOutlined";
import { APIKeyWithOwner } from "api/typesGenerated";
import { css } from "@emotion/react";

export const TokensPage: FC<PropsWithChildren<unknown>> = () => {
  const cliCreateCommand = "coder tokens create";

  const TokenActions = () => (
    <Stack direction="row" justifyContent="end" css={{ marginBottom: 8 }}>
      <Button startIcon={<AddIcon />} component={RouterLink} to="new">
        Add token
      </Button>
    </Stack>
  );

  const [tokenToDelete, setTokenToDelete] = useState<
    APIKeyWithOwner | undefined
  >(undefined);

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
  });

  return (
    <>
      <Section
        title="Tokens"
        css={(theme) => css`
          & code {
            background: ${theme.palette.divider};
            font-size: 12px;
            padding: 2px 4px;
            color: ${theme.palette.text.primary};
            border-radius: 2px;
          }
        `}
        description={
          <>
            Tokens are used to authenticate with the Coder API. You can create a
            token with the Coder CLI using the <code>{cliCreateCommand}</code>{" "}
            command.
          </>
        }
        layout="fluid"
      >
        <TokenActions />
        <TokensPageView
          tokens={tokens}
          isLoading={isFetching}
          hasLoaded={isFetched}
          getTokensError={getTokensError}
          onDelete={(token) => {
            setTokenToDelete(token);
          }}
        />
      </Section>
      <ConfirmDeleteDialog
        queryKey={queryKey}
        token={tokenToDelete}
        setToken={setTokenToDelete}
      />
    </>
  );
};

export default TokensPage;
