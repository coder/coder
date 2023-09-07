import { FC, useState } from "react";
import { useTranslation } from "react-i18next";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm";
import { useNavigate } from "react-router-dom";
import { useFormik } from "formik";
import { Loader } from "components/Loader/Loader";
import { displaySuccess, displayError } from "components/GlobalSnackbar/utils";
import { useMutation, useQuery } from "@tanstack/react-query";
import { createToken, getTokenConfig } from "api/api";
import { CreateTokenForm } from "./CreateTokenForm";
import { NANO_HOUR, CreateTokenData } from "./utils";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { CodeExample } from "components/CodeExample/CodeExample";
import { makeStyles } from "@mui/styles";
import { ErrorAlert } from "components/Alert/ErrorAlert";

const initialValues: CreateTokenData = {
  name: "",
  lifetime: 30,
};

export const CreateTokenPage: FC = () => {
  const { t } = useTranslation("tokensPage");
  const styles = useStyles();
  const navigate = useNavigate();

  const {
    mutate: saveToken,
    isLoading: isCreating,
    isError: creationFailed,
    isSuccess: creationSuccessful,
    data: newToken,
  } = useMutation(createToken);
  const {
    data: tokenConfig,
    isLoading: fetchingTokenConfig,
    isError: tokenFetchFailed,
    error: tokenFetchError,
  } = useQuery({
    queryKey: ["tokenconfig"],
    queryFn: getTokenConfig,
  });

  const [formError, setFormError] = useState<unknown>(undefined);

  const onCreateSuccess = () => {
    displaySuccess(t("createToken.createSuccess"));
    navigate("/settings/tokens");
  };

  const onCreateError = (error: unknown) => {
    setFormError(error);
    displayError(t("createToken.createError"));
  };

  const form = useFormik<CreateTokenData>({
    initialValues,
    onSubmit: (values) => {
      saveToken(
        {
          lifetime: values.lifetime * 24 * NANO_HOUR,
          token_name: values.name,
          scope: "all", // tokens are currently unscoped
        },
        {
          onError: onCreateError,
        },
      );
    },
  });

  const tokenDescription = (
    <>
      <p>{t("createToken.successModal.description")}</p>
      <CodeExample code={newToken?.key ?? ""} className={styles.codeExample} />
    </>
  );

  if (fetchingTokenConfig) {
    return <Loader />;
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle("Create Token")}</title>
      </Helmet>
      {tokenFetchFailed && <ErrorAlert error={tokenFetchError} />}
      <FullPageHorizontalForm
        title={t("createToken.title")}
        detail={t("createToken.detail")}
      >
        <CreateTokenForm
          form={form}
          maxTokenLifetime={tokenConfig?.max_token_lifetime}
          formError={formError}
          setFormError={setFormError}
          isCreating={isCreating}
          creationFailed={creationFailed}
        />

        <ConfirmDialog
          type="info"
          hideCancel
          title={t("createToken.successModal.title")}
          description={tokenDescription}
          open={creationSuccessful && Boolean(newToken.key)}
          confirmLoading={isCreating}
          onConfirm={onCreateSuccess}
          onClose={onCreateSuccess}
        />
      </FullPageHorizontalForm>
    </>
  );
};

const useStyles = makeStyles((theme) => ({
  codeExample: {
    minHeight: "auto",
    userSelect: "all",
    width: "100%",
    marginTop: theme.spacing(3),
  },
}));

export default CreateTokenPage;
