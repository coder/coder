import { FC, PropsWithChildren, useState } from "react";
import { Section } from "components/SettingsLayout/Section";
import { TokensPageView } from "./TokensPageView";
import makeStyles from "@mui/styles/makeStyles";
import { useTranslation } from "react-i18next";
import { useTokensData } from "./hooks";
import { ConfirmDeleteDialog } from "./ConfirmDeleteDialog";
import { Stack } from "components/Stack/Stack";
import Button from "@mui/material/Button";
import { Link as RouterLink } from "react-router-dom";
import AddIcon from "@mui/icons-material/AddOutlined";
import { APIKeyWithOwner } from "api/typesGenerated";

export const TokensPage: FC<PropsWithChildren<unknown>> = () => {
  const styles = useStyles();
  const { t } = useTranslation("tokensPage");

  const cliCreateCommand = "coder tokens create";

  const TokenActions = () => (
    <Stack direction="row" justifyContent="end" className={styles.tokenActions}>
      <Button startIcon={<AddIcon />} component={RouterLink} to="new">
        {t("tokenActions.addToken")}
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
        title={t("title")}
        className={styles.section}
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
}));

export default TokensPage;
