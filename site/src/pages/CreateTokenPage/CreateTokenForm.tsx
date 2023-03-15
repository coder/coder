import { FC, useState, useEffect } from "react"
import {
  FormFields,
  FormSection,
  FormFooter,
  HorizontalForm,
} from "components/Form/Form"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { useTranslation } from "react-i18next"
import { onChangeTrimmed, getFormHelpers } from "util/formUtils"
import TextField from "@material-ui/core/TextField"
import MenuItem from "@material-ui/core/MenuItem"
import {
  NANO_HOUR,
  CreateTokenData,
  determineDefaultLtValue,
  filterByMaxTokenLifetime,
  customLifetimeDay,
} from "./utils"
import { FormikContextType } from "formik"
import dayjs from "dayjs"
import { useNavigate } from "react-router-dom"
import { Stack } from "components/Stack/Stack"

interface CreateTokenFormProps {
  form: FormikContextType<CreateTokenData>
  maxTokenLifetime?: number
  formError: Error | unknown
  setFormError: (arg0: Error | unknown) => void
  isCreating: boolean
  creationFailed: boolean
}

export const CreateTokenForm: FC<CreateTokenFormProps> = ({
  form,
  maxTokenLifetime,
  formError,
  setFormError,
  isCreating,
  creationFailed,
}) => {
  const styles = useStyles()
  const { t } = useTranslation("tokensPage")
  const navigate = useNavigate()

  const [expDays, setExpDays] = useState<number>(1)
  const [lifetimeDays, setLifetimeDays] = useState<number | string>(
    determineDefaultLtValue(maxTokenLifetime),
  )

  useEffect(() => {
    if (lifetimeDays !== "custom") {
      void form.setFieldValue("lifetime", lifetimeDays)
    } else {
      void form.setFieldValue("lifetime", expDays)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- adding form will cause an infinite loop
  }, [lifetimeDays, expDays])

  const getFieldHelpers = getFormHelpers<CreateTokenData>(form, formError)

  return (
    <HorizontalForm onSubmit={form.handleSubmit}>
      <FormSection
        title={t("createToken.nameSection.title")}
        description={t("createToken.nameSection.description")}
        classes={{ sectionInfo: styles.formSectionInfo }}
      >
        <FormFields>
          <TextField
            {...getFieldHelpers("name")}
            label={t("createToken.fields.name")}
            required
            onChange={onChangeTrimmed(form, () => setFormError(undefined))}
            autoFocus
            fullWidth
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
        classes={{ sectionInfo: styles.formSectionInfo }}
      >
        <FormFields>
          <Stack direction="row">
            <TextField
              select
              label={t("createToken.fields.lifetime")}
              required
              defaultValue={determineDefaultLtValue(maxTokenLifetime)}
              onChange={(event) => {
                void setLifetimeDays(event.target.value)
              }}
              fullWidth
              InputLabelProps={{
                shrink: true,
              }}
            >
              {filterByMaxTokenLifetime(maxTokenLifetime).map((lt) => (
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

            {lifetimeDays === "custom" && (
              <TextField
                type="date"
                label={t("createToken.lifetimeSection.expiresOn")}
                defaultValue={dayjs().add(expDays, "day").format("YYYY-MM-DD")}
                onChange={(event) => {
                  const lt = Math.ceil(
                    dayjs(event.target.value).diff(dayjs(), "day", true),
                  )
                  setExpDays(lt)
                }}
                inputProps={{
                  min: dayjs().add(1, "day").format("YYYY-MM-DD"),
                  max: maxTokenLifetime
                    ? dayjs()
                        .add(maxTokenLifetime / NANO_HOUR / 24, "day")
                        .format("YYYY-MM-DD")
                    : undefined,
                  required: true,
                }}
                fullWidth
                InputLabelProps={{
                  shrink: true,
                  required: true,
                }}
              />
            )}
          </Stack>
        </FormFields>
      </FormSection>
      <FormFooter
        onCancel={() => navigate("/settings/tokens")}
        isLoading={isCreating}
        submitLabel={
          creationFailed
            ? t("createToken.footer.retry")
            : t("createToken.footer.submit")
        }
      />
    </HorizontalForm>
  )
}

const useStyles = makeStyles(() => ({
  formSectionInfo: {
    minWidth: "300px",
  },
}))
