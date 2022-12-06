import TextField from "@material-ui/core/TextField"
import { useActor } from "@xstate/react"
import { FeatureNames } from "api/types"
import {
  Badges,
  DisabledBadge,
  EnterpriseBadge,
  EntitledBadge,
} from "components/DeploySettingsLayout/Badges"
import { Header } from "components/DeploySettingsLayout/Header"
import { LoadingButton } from "components/LoadingButton/LoadingButton"
import { Stack } from "components/Stack/Stack"
import { FormikContextType, useFormik } from "formik"
import React, { useContext, useState } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import * as Yup from "yup"
import { XServiceContext } from "xServices/StateContext"
import { getFormHelpers } from "util/formUtils"
import makeStyles from "@material-ui/core/styles/makeStyles"
import FormControlLabel from "@material-ui/core/FormControlLabel"
import Switch from "@material-ui/core/Switch"
import { BlockPicker } from "react-color"
import { useTheme } from "@material-ui/core/styles"
import FormHelperText from "@material-ui/core/FormHelperText"
import Button from "@material-ui/core/Button"
import { useTranslation } from "react-i18next"

export interface ServiceBannerFormValues {
  message?: string
  backgroundColor?: string
  enabled: boolean
}

// TODO:
const validationSchema = Yup.object({})

// ServiceBanner is unlike the other Deployment Settings pages because it
// implements a form, whereas the others are read-only. We make this
// exception because the Service Banner is visual, and configuring it from
// the command line would be a significantly worse user experience.
const ServiceBannerSettingsPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [serviceBannerState, serviceBannerSend] = useActor(
    xServices.serviceBannerXService,
  )

  const [entitlementsState] = useActor(xServices.entitlementsXService)

  const serviceBanner = serviceBannerState.context.serviceBanner

  const styles = useStyles()

  const isEntitled =
    entitlementsState.context.entitlements.features[FeatureNames.ServiceBanners]
      .entitlement !== "not_entitled"

  const setBanner = (values: ServiceBannerFormValues, preview: boolean) => {
    const newBanner = {
      message: values.message,
      enabled: values.enabled,
      background_color: values.backgroundColor,
    }
    if (preview) {
      serviceBannerSend({
        type: "SET_PREVIEW_BANNER",
        serviceBanner: newBanner,
      })
      return
    }
    serviceBannerSend({
      type: "SET_BANNER",
      serviceBanner: newBanner,
    })
  }

  const initialValues: ServiceBannerFormValues = {
    message: serviceBanner.message,
    enabled: serviceBanner.enabled,
    backgroundColor: serviceBanner.background_color,
  }

  const form: FormikContextType<ServiceBannerFormValues> =
    useFormik<ServiceBannerFormValues>({
      initialValues,
      validationSchema,
      onSubmit: (values) => setBanner(values, false),
    })
  const getFieldHelpers = getFormHelpers<ServiceBannerFormValues>(form)

  const [backgroundColor, setBackgroundColor] = useState(
    form.values.backgroundColor,
  )

  const theme = useTheme()
  const [t] = useTranslation("serviceBannerSettings")

  return (
    <>
      <Helmet>
        <title>{pageTitle("Service Banner Settings")}</title>
      </Helmet>

      <Header
        title="Service Banner"
        description="Configure a banner that displays a message to all users"
        docsHref="https://coder.com/docs/coder-oss/latest/admin/service-banners"
      />
      <Badges>
        {isEntitled ? <EntitledBadge /> : <DisabledBadge />}
        <EnterpriseBadge />
      </Badges>

      {isEntitled ? (
        <form className={styles.form} onSubmit={form.handleSubmit}>
          <Stack>
            <FormControlLabel
              control={
                <Switch
                  color="primary"
                  checked={form.values.enabled}
                  onChange={() => {
                    const newState = !form.values.enabled
                    const newBanner = {
                      ...form.values,
                      enabled: newState,
                    }
                    setBanner(newBanner, false)
                    form.setFieldValue("enabled", newState)
                  }}
                />
              }
              label="Enabled"
            />
            <Stack spacing={0}>
              <TextField
                {...getFieldHelpers("message")}
                fullWidth
                label="Message"
                variant="outlined"
                multiline
                onChange={(e) => {
                  form.setFieldValue("message", e.target.value)
                  setBanner(
                    {
                      ...form.values,
                      message: e.target.value,
                    },
                    true,
                  )
                }}
              />
              <FormHelperText>{t("messageHelperText")}</FormHelperText>
            </Stack>

            <Stack spacing={0}>
              <h3>{"Background Color"}</h3>
              <BlockPicker
                color={backgroundColor}
                onChange={(color) => {
                  setBackgroundColor(color.hex)
                  form.setFieldValue("backgroundColor", color.hex)
                  setBanner(
                    {
                      ...form.values,
                      backgroundColor: color.hex,
                    },
                    true,
                  )
                }}
                triangle="hide"
                colors={["#004852", "#D65D0F", "#4CD473", "#D94A5D", "#5A00CF"]}
                styles={{
                  default: {
                    input: {
                      color: "white",
                      backgroundColor: theme.palette.background.default,
                    },
                    body: {
                      backgroundColor: "black",
                      color: "white",
                    },
                    card: {
                      backgroundColor: "black",
                    },
                  },
                }}
              />
            </Stack>

            <Stack direction="row">
              <LoadingButton loading={false} type="submit" variant="contained">
                {t("updateLabel")}
              </LoadingButton>
            </Stack>
          </Stack>
        </form>
      ) : (
        <>
          <p>
            Your license does not include Service Banners.{" "}
            <a href="mailto:sales@coder.com">Contact sales</a> to learn more.
          </p>
          <Button
            onClick={() => {
              setBanner(
                {
                  message:
                    "👋 **This** is a service banner. The banner's color and text are editable.",
                  backgroundColor: "#004852",
                  enabled: true,
                },
                true,
              )
            }}
          >
            {t("showPreviewLabel")}
          </Button>
        </>
      )}
    </>
  )
}

const useStyles = makeStyles(() => ({
  form: {
    maxWidth: "500px",
  },
}))

export default ServiceBannerSettingsPage
