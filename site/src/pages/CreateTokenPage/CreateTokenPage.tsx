import { FC, useState } from "react"
import { useTranslation } from "react-i18next"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm"
import { useNavigate } from "react-router-dom"
import {
  FormFields,
  FormSection,
  FormFooter,
  HorizontalForm,
} from "components/HorizontalForm/HorizontalForm"
import { useFormik } from "formik"
import { getFormHelpers, onChangeTrimmed } from "util/formUtils"
import TextField from "@material-ui/core/TextField"
import MenuItem from "@material-ui/core/MenuItem"
import { displaySuccess, displayError } from "components/GlobalSnackbar/utils"
import { useMutation } from "@tanstack/react-query"
import { createToken } from "api/api"

const NANO_HOUR = 3600000000000

const lifetimes = [
  { label: "7 days", lifetimeDays: 7 },
  { label: "30 days", lifetimeDays: 30 },
  { label: "60 days", lifetimeDays: 60 },
  { label: "90 days", lifetimeDays: 60 },
]

interface CreateTokenData {
  name: string
  lifetime: number
}

const initialValues: CreateTokenData = {
  name: "",
  lifetime: 30,
}

const CreateTokenPage: FC = () => {
  const { t } = useTranslation("tokensPage")
  const navigate = useNavigate()
  const navigateBack = () => navigate(-1)
  const useCreateToken = () => useMutation(createToken)
  const [formErr, setFormErr] = useState<unknown | undefined>(undefined)

  const { mutate: saveToken, isLoading, isError } = useCreateToken()

  const onCreateSuccess = () => {
    displaySuccess(t("createToken.createSuccess"))
    navigateBack()
  }

  const onCreateError = (error: unknown) => {
    setFormErr(error)
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

  const getFieldHelpers = getFormHelpers<CreateTokenData>(form, formErr)

  return (
    <>
      <Helmet>
        <title>{pageTitle(t("createToken.title"))}</title>
      </Helmet>
      <FullPageHorizontalForm
        title={t("createToken.title")}
        detail={t("createToken.detail")}
      >
        <HorizontalForm onSubmit={form.handleSubmit}>
          <FormSection
            title={t("createToken.nameSection.title")}
            description={t("createToken.nameSection.description")}
          >
            <FormFields>
              <TextField
                {...getFieldHelpers("name")}
                onChange={onChangeTrimmed(form)}
                autoFocus
                fullWidth
                required
                label={t("createToken.fields.name")}
                variant="outlined"
              />
            </FormFields>
          </FormSection>
          <FormSection
            title={t("createToken.lifetimeSection.title")}
            description={t("createToken.lifetimeSection.description")}
          >
            <FormFields>
              <TextField
                {...getFieldHelpers("lifetime")}
                InputLabelProps={{
                  shrink: true,
                }}
                label={t("createToken.fields.lifetime")}
                select
                required
                autoFocus
                fullWidth
              >
                {lifetimes.map((lifetime) => (
                  <MenuItem key={lifetime.label} value={lifetime.lifetimeDays}>
                    {lifetime.label}
                  </MenuItem>
                ))}
              </TextField>
            </FormFields>
          </FormSection>
          <FormFooter
            onCancel={navigateBack}
            isLoading={isLoading}
            submitLabel={
              isError
                ? t("createToken.footer.retry")
                : t("createToken.footer.submit")
            }
          />
        </HorizontalForm>
      </FullPageHorizontalForm>
    </>
  )
}

export default CreateTokenPage
