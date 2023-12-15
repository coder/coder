import { css, type Interpolation, type Theme } from "@emotion/react";
import { type FC, useState } from "react";
import { Link as RouterLink } from "react-router-dom";
import Button from "@mui/material/Button";
import AddIcon from "@mui/icons-material/AddOutlined";
import type { APIKeyWithOwner } from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import { Section } from "../Section";
import { useTokensData } from "./hooks";
import { TokensPageView } from "./TokensPageView";
import { ConfirmDeleteDialog } from "./ConfirmDeleteDialog";

const cliCreateCommand = "coder tokens create";

export const TokensPage: FC = () => {
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
        css={styles.section}
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

const TokenActions: FC = () => (
  <Stack direction="row" justifyContent="end" css={{ marginBottom: 8 }}>
    <Button startIcon={<AddIcon />} component={RouterLink} to="new">
      Add token
    </Button>
  </Stack>
);

const styles = {
  section: (theme) => css`
    & code {
      background: ${theme.palette.divider};
      font-size: 12px;
      padding: 2px 4px;
      color: ${theme.palette.text.primary};
      border-radius: 2px;
    }
  `,
} satisfies Record<string, Interpolation<Theme>>;

export default TokensPage;
