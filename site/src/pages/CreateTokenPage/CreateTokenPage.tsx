import { useFormik } from "formik";
import { type FC, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery } from "react-query";
import { useNavigate } from "react-router-dom";
import { createToken, getTokenConfig } from "api/api";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { CodeExample } from "components/CodeExample/CodeExample";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm";
import { displaySuccess, displayError } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { pageTitle } from "utils/page";
import { CreateTokenForm } from "./CreateTokenForm";
import { type CreateTokenData, NANO_HOUR } from "./utils";

const initialValues: CreateTokenData = {
  name: "",
  lifetime: 30,
};

export const CreateTokenPage: FC = () => {
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
    displaySuccess("Token has been created");
    navigate("/settings/tokens");
  };

  const onCreateError = (error: unknown) => {
    setFormError(error);
    displayError("Failed to create token");
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
      <p>Make sure you copy the below token before proceeding:</p>
      <CodeExample
        secret={false}
        code={newToken?.key ?? ""}
        css={{
          minHeight: "auto",
          userSelect: "all",
          width: "100%",
          marginTop: 24,
        }}
      />
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
        title="Create Token"
        detail="All tokens are unscoped and therefore have full resource access."
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
          title="Creation successful"
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

export default CreateTokenPage;
