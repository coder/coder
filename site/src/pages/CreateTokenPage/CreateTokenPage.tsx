import { FC, useState } from "react"
import { useTranslation } from "react-i18next"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm"
import { useNavigate } from "react-router-dom"
import { useFormik } from "formik"
import { Loader } from "components/Loader/Loader"
import { displaySuccess, displayError } from "components/GlobalSnackbar/utils"
import { useMutation, useQuery } from "@tanstack/react-query"
import { createToken, getTokenConfig } from "api/api"
import { CreateTokenForm } from "./CreateTokenForm"
import { NANO_HOUR, CreateTokenData } from "./utils"
import { AlertBanner } from "components/AlertBanner/AlertBanner"

const initialValues: CreateTokenData = {
  name: "",
  lifetime: 30,
}

const CreateTokenPage: FC = () => {
  const { t } = useTranslation("tokensPage")
  const navigate = useNavigate()

  const {
    mutate: saveToken,
    isLoading: isCreating,
    isError: creationFailed,
  } = useMutation(createToken)
  const {
    data: tokenConfig,
    isLoading: fetchingTokenConfig,
    isError: tokenFetchFailed,
    error: tokenFetchError,
  } = useQuery({
    queryKey: ["tokenconfig"],
    queryFn: getTokenConfig,
  })

  const [formError, setFormError] = useState<unknown | undefined>(undefined)

  const onCreateSuccess = () => {
    displaySuccess(t("createToken.createSuccess"))
    navigate("/settings/tokens")
  }

  const onCreateError = (error: unknown) => {
    setFormError(error)
    displayError(t("createToken.createError"))
  }

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
          onSuccess: onCreateSuccess,
        },
      )
    },
  })

  if (fetchingTokenConfig) {
    return <Loader />
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle(t("createToken.title"))}</title>
      </Helmet>
      {tokenFetchFailed && (
        <AlertBanner severity="error" error={tokenFetchError} />
      )}
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
      </FullPageHorizontalForm>
    </>
  )
}

export default CreateTokenPage
