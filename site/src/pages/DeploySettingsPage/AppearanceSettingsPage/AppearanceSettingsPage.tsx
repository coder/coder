import Button from "@material-ui/core/Button"
import FormControlLabel from "@material-ui/core/FormControlLabel"
import FormHelperText from "@material-ui/core/FormHelperText"
import InputAdornment from "@material-ui/core/InputAdornment"
import { useTheme } from "@material-ui/core/styles"
import makeStyles from "@material-ui/core/styles/makeStyles"
import Switch from "@material-ui/core/Switch"
import TextField from "@material-ui/core/TextField"
import { useActor } from "@xstate/react"
import { FeatureNames } from "api/types"
import { AppearanceConfig } from "api/typesGenerated"
import {
  Badges,
  DisabledBadge,
  EnterpriseBadge,
  EntitledBadge,
} from "components/DeploySettingsLayout/Badges"
import { Fieldset } from "components/DeploySettingsLayout/Fieldset"
import { Header } from "components/DeploySettingsLayout/Header"
import { Stack } from "components/Stack/Stack"
import { useFormik } from "formik"
import React, { useContext, useState } from "react"
import { BlockPicker } from "react-color"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { getFormHelpers } from "util/formUtils"
import { pageTitle } from "util/page"
import { XServiceContext } from "xServices/StateContext"

// ServiceBanner is unlike the other Deployment Settings pages because it
// implements a form, whereas the others are read-only. We make this
// exception because the Service Banner is visual, and configuring it from
// the command line would be a significantly worse user experience.
const AppearanceSettingsPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [appearanceXService, appearanceSend] = useActor(
    xServices.appearanceXService,
  )
  const [entitlementsState] = useActor(xServices.entitlementsXService)
  const appearance = appearanceXService.context.appearance
  const styles = useStyles()

  const isEntitled =
    entitlementsState.context.entitlements.features[FeatureNames.Appearance]
      .entitlement !== "not_entitled"

  const updateAppearance = (
    newConfig: Partial<AppearanceConfig>,
    preview: boolean,
  ) => {
    const newAppearance = {
      ...appearance,
      ...newConfig,
    }
    if (preview) {
      appearanceSend({
        type: "SET_PREVIEW_APPEARANCE",
        appearance: newAppearance,
      })
      return
    }
    appearanceSend({
      type: "SET_APPEARANCE",
      appearance: newAppearance,
    })
  }

  const logoForm = useFormik<{
    logo_url: string
  }>({
    initialValues: {
      logo_url: appearance.logo_url,
    },
    onSubmit: (values) => updateAppearance(values, false),
  })
  const logoFieldHelpers = getFormHelpers(logoForm)

  const serviceBannerForm = useFormik<AppearanceConfig["service_banner"]>({
    initialValues: {
      message: appearance.service_banner.message,
      enabled: appearance.service_banner.enabled,
      background_color: appearance.service_banner.background_color,
    },
    onSubmit: (values) =>
      updateAppearance(
        {
          service_banner: values,
        },
        false,
      ),
  })
  const serviceBannerFieldHelpers = getFormHelpers(serviceBannerForm)
  const [backgroundColor, setBackgroundColor] = useState(
    serviceBannerForm.values.background_color,
  )

  const theme = useTheme()
  const [t] = useTranslation("appearanceSettings")

  return (
    <>
      <Helmet>
        <title>{pageTitle("Appearance Settings")}</title>
      </Helmet>

      <Header
        title="Appearance"
        description="Customize the look and feel of your Coder deployment."
      />

      <Badges>
        {isEntitled ? <EntitledBadge /> : <DisabledBadge />}
        <EnterpriseBadge />
      </Badges>

      <Fieldset
        title="Logo URL"
        validation={
          isEntitled
            ? "We recommend a transparent image with 3:1 aspect ratio."
            : "This is an Enterprise only feature."
        }
        onSubmit={logoForm.handleSubmit}
        button={!isEntitled && <Button disabled>Submit</Button>}
      >
        <p>
          Specify a custom URL for your logo to be displayed in the top left
          corner of the dashboard.
        </p>
        <TextField
          {...logoFieldHelpers("logo_url")}
          defaultValue={appearance.logo_url}
          fullWidth
          placeholder="Leave empty to display the Coder logo."
          disabled={!isEntitled}
          InputProps={{
            endAdornment: (
              <InputAdornment position="end" className={styles.logoAdornment}>
                <img
                  alt=""
                  src={logoForm.values.logo_url}
                  // This prevent browser to display the ugly error icon if the
                  // image path is wrong or user didn't finish typing the url
                  onError={(e) => (e.currentTarget.style.display = "none")}
                  onLoad={(e) => (e.currentTarget.style.display = "inline")}
                />
              </InputAdornment>
            ),
          }}
        />
      </Fieldset>

      <Fieldset
        title="Service Banner"
        onSubmit={serviceBannerForm.handleSubmit}
        button={
          !isEntitled && (
            <Button
              onClick={() => {
                updateAppearance(
                  {
                    service_banner: {
                      message:
                        "👋 **This** is a service banner. The banner's color and text are editable.",
                      background_color: "#004852",
                      enabled: true,
                    },
                  },
                  true,
                )
              }}
            >
              {t("showPreviewLabel")}
            </Button>
          )
        }
        validation={
          !isEntitled && (
            <p>
              Your license does not include Service Banners.{" "}
              <a href="mailto:sales@coder.com">Contact sales</a> to learn more.
            </p>
          )
        }
      >
        <p>Configure a banner that displays a message to all users.</p>

        {isEntitled && (
          <Stack>
            <FormControlLabel
              control={
                <Switch
                  color="primary"
                  checked={serviceBannerForm.values.enabled}
                  onChange={async () => {
                    const newState = !serviceBannerForm.values.enabled
                    const newBanner = {
                      ...serviceBannerForm.values,
                      enabled: newState,
                    }
                    updateAppearance(
                      {
                        service_banner: newBanner,
                      },
                      false,
                    )
                    await serviceBannerForm.setFieldValue("enabled", newState)
                  }}
                />
              }
              label="Enabled"
            />
            <Stack spacing={0}>
              <TextField
                {...serviceBannerFieldHelpers("message")}
                fullWidth
                label="Message"
                variant="outlined"
                multiline
              />
              <FormHelperText>{t("messageHelperText")}</FormHelperText>
            </Stack>

            <Stack spacing={0}>
              <h3>{"Background Color"}</h3>
              <BlockPicker
                color={backgroundColor}
                onChange={async (color) => {
                  setBackgroundColor(color.hex)
                  await serviceBannerForm.setFieldValue(
                    "background_color",
                    color.hex,
                  )
                  updateAppearance(
                    {
                      service_banner: {
                        ...serviceBannerForm.values,
                        background_color: color.hex,
                      },
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
          </Stack>
        )}
      </Fieldset>
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  form: {
    maxWidth: "500px",
  },
  logoAdornment: {
    width: theme.spacing(3),
    height: theme.spacing(3),

    "& img": {
      maxWidth: "100%",
    },
  },
}))

export default AppearanceSettingsPage
