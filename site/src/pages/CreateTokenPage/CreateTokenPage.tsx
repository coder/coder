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
import { useMutation, useQuery } from "@tanstack/react-query"
import { createToken, getTokenConfig } from "api/api"
import i18next from "i18next"
import dayjs from "dayjs"
import makeStyles from "@material-ui/core/styles/makeStyles"

const NANO_HOUR = 3600000000000

interface LifetimeDay {
  label: string
  value: number | string
}

const lifetimeDayPresets: LifetimeDay[] = [
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

  // {
  //   label: i18next.t("tokensPage:createToken.lifetimeSection.noExpiration"),
  //   value: 365 * 290, // fix
  // },
]

const customLifetimeDay: LifetimeDay = {
  label: i18next.t("tokensPage:createToken.lifetimeSection.custom"),
  value: "custom",
}

interface CreateTokenData {
  name: string
  lifetime: number
}

const initialValues: CreateTokenData = {
  name: "",
  lifetime: 30,
}

const filterByMaxTokenLifetime = (
  ltArr: LifetimeDay[],
  maxTokenLifetime?: number,
): LifetimeDay[] => {
  // if maxTokenLifetime hasn't been set, return the full array of options
  if (!maxTokenLifetime) {
    return ltArr
  }

  // otherwise only return options that are less than or equal to the max lifetime
  return ltArr.filter(
    (lifetime) => lifetime.value <= maxTokenLifetime / NANO_HOUR / 24,
  )
}

const determineDefaultLtValue = (maxTokenLifetime?: number) => {
  const filteredArr = filterByMaxTokenLifetime(
    lifetimeDayPresets,
    maxTokenLifetime,
  )

  // default to a lifetime of 30 days if within the maxTokenLifetime
  const thirtyDayDefault = filteredArr.find((lt) => lt.value === 30)
  if (thirtyDayDefault) {
    return thirtyDayDefault.value
  }

  // otherwise default to the first preset option
  if (filteredArr[0]) {
    return filteredArr[0].value
  }

  // if no preset options are within the maxTokenLifetime, default to "custom"
  return "custom"
}

const CreateTokenPage: FC = () => {
  const styles = useStyles()
  const { t } = useTranslation("tokensPage")
  const navigate = useNavigate()

  const { mutate: saveToken, isLoading, isError } = useMutation(createToken)
  const { data: tokenConfig } = useQuery({
    queryKey: ["tokenconfig"],
    queryFn: getTokenConfig,
  })

  const [formError, setFormError] = useState<unknown | undefined>(undefined)
  const [expDays, setExpDays] = useState<number>(1)
  const [lifetimeDays, setLifetimeDays] = useState<number | string>(
    determineDefaultLtValue(tokenConfig?.max_token_lifetime),
  )

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
                label={t("createToken.fields.name")}
                required
                onChange={onChangeTrimmed(form, () => setFormError(undefined))}
                autoFocus
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
                select
                label={t("createToken.fields.lifetime")}
                required
                defaultValue={determineDefaultLtValue(
                  tokenConfig?.max_token_lifetime,
                )}
                onChange={(event) => {
                  void setLifetimeDays(event.target.value)
                }}
                InputLabelProps={{
                  shrink: true,
                }}
              >
                {filterByMaxTokenLifetime(
                  lifetimeDayPresets,
                  tokenConfig?.max_token_lifetime,
                ).map((lt) => (
                  <MenuItem key={lt.label} value={lt.value}>
                    {lt.label}
                  </MenuItem>
                ))}
                <MenuItem
                  key={customLifetimeDay.label}
                  value={customLifetimeDay.value}
                >
                  {customLifetimeDay.label}
                </MenuItem>
              </TextField>
            </FormFields>
            <FormFields>
              {lifetimeDays === "custom" && (
                <TextField
                  type="date"
                  label={t("createToken.lifetimeSection.expiresOn")}
                  defaultValue={dayjs()
                    .add(expDays, "day")
                    .format("YYYY-MM-DD")}
                  onChange={(event) => {
                    const lt = Math.ceil(
                      dayjs(event.target.value).diff(dayjs(), "day", true),
                    )
                    setExpDays(lt)
                  }}
                  inputProps={{
                    min: dayjs().add(1, "day").format("YYYY-MM-DD"),
                    max: tokenConfig?.max_token_lifetime
                      ? dayjs()
                          .add(
                            tokenConfig.max_token_lifetime / NANO_HOUR / 24,
                            "day",
                          )
                          .format("YYYY-MM-DD")
                      : undefined,
                    required: true,
                  }}
                  InputLabelProps={{
                    shrink: true,
                    required: true,
                  }}
                  className={styles.expField}
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
