import { FC, useEffect, useState } from "react"
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
import i18next from "i18next"
import dayjs from "dayjs"
import makeStyles from "@material-ui/core/styles/makeStyles"

const NANO_HOUR = 3600000000000

const lifetimeDayArr = [
  {
    label: i18next.t("tokensPage:createToken.lifetimeSection.7"),
    value: 7,
  },
  {
    label: i18next.t("tokensPage:createToken.lifetimeSection.30"),
    value: 30,
  },
  {
    label: i18next.t("tokensPage:createToken.lifetimeSection.60"),
    value: 60,
  },
  {
    label: i18next.t("tokensPage:createToken.lifetimeSection.90"),
    value: 90,
  },
  {
    label: i18next.t("tokensPage:createToken.lifetimeSection.custom"),
    value: "custom",
  },
  // {
  //   label: i18next.t("tokensPage:createToken.lifetimeSection.noExpiration"),
  //   value: 365 * 290, // fix
  // },
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
  const styles = useStyles()
  const { t } = useTranslation("tokensPage")
  const navigate = useNavigate()

  const useCreateToken = () => useMutation(createToken)

  const [formError, setFormError] = useState<unknown | undefined>(undefined)
  const [lifetimeDays, setLifetimeDays] = useState<number | string>(30)
  const [expDays, setExpDays] = useState<number>(1)

  const { mutate: saveToken, isLoading, isError } = useCreateToken()

  useEffect(() => {
    if (lifetimeDays !== "custom") {
      void form.setFieldValue("lifetime", lifetimeDays)
    } else {
      void form.setFieldValue("lifetime", expDays)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- adding form will cause an infinite loop
  }, [lifetimeDays, expDays])

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

  const getFieldHelpers = getFormHelpers<CreateTokenData>(form, formError)

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
            className={styles.formSection}
          >
            <FormFields>
              <TextField
                {...getFieldHelpers("name")}
                onChange={onChangeTrimmed(form, () => setFormError(undefined))}
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
            description={
              form.values.lifetime
                ? t("createToken.lifetimeSection.description", {
                    date: dayjs()
                      .add(form.values.lifetime, "days")
                      .utc()
                      .format("MMMM DD, YYYY"),
                  })
                : t("createToken.lifetimeSection.emptyDescription")
            }
            className={styles.formSection}
          >
            <FormFields>
              <TextField
                onChange={(event) => {
                  void setLifetimeDays(event.target.value)
                }}
                InputLabelProps={{
                  shrink: true,
                }}
                label={t("createToken.fields.lifetime")}
                select
                defaultValue={30}
                required
                autoFocus
              >
                {lifetimeDayArr.map((lt) => (
                  <MenuItem key={lt.label} value={lt.value}>
                    {lt.label}
                  </MenuItem>
                ))}
              </TextField>
            </FormFields>
            <FormFields>
              {lifetimeDays === "custom" && (
                <TextField
                  onChange={(event) => {
                    const lt = Math.ceil(
                      dayjs(event.target.value).diff(dayjs(), "day", true),
                    )
                    setExpDays(lt)
                  }}
                  label={t("createToken.lifetimeSection.expiresOn")}
                  type="date"
                  className={styles.expField}
                  defaultValue={dayjs()
                    .add(expDays, "day")
                    .format("YYYY-MM-DD")}
                  autoFocus
                  inputProps={{
                    min: dayjs().add(1, "day").format("YYYY-MM-DD"),
                    required: true,
                  }}
                  InputLabelProps={{
                    shrink: true,
                    required: true,
                  }}
                />
              )}
            </FormFields>
          </FormSection>
          <FormFooter
            onCancel={() => navigate("/settings/tokens")}
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

const useStyles = makeStyles((theme) => ({
  formSection: {
    gap: 0,
  },
  expField: {
    marginLeft: theme.spacing(2),
  },
}))

export default CreateTokenPage
