import { useState } from "react"
import { Header } from "components/DeploySettingsLayout/Header"
import {
  Badges,
  DisabledBadge,
  EnterpriseBadge,
  EntitledBadge,
} from "components/DeploySettingsLayout/Badges"
import InputAdornment from "@material-ui/core/InputAdornment"
import { Fieldset } from "components/DeploySettingsLayout/Fieldset"
import { getFormHelpers } from "util/formUtils"
import Button from "@material-ui/core/Button"
import FormControlLabel from "@material-ui/core/FormControlLabel"
import FormHelperText from "@material-ui/core/FormHelperText"
import { BlockPicker } from "react-color"
import { useTranslation } from "react-i18next"
import makeStyles from "@material-ui/core/styles/makeStyles"
import Switch from "@material-ui/core/Switch"
import TextField from "@material-ui/core/TextField"
import { AppearanceConfig } from "api/typesGenerated"
import { Stack } from "components/Stack/Stack"
import { useFormik } from "formik"
import { useTheme } from "@material-ui/core/styles"

export type AppearanceSettingsPageViewProps = {
  appearance: AppearanceConfig
  isEntitled: boolean
  updateAppearance: (
    newConfig: Partial<AppearanceConfig>,
    preview: boolean,
  ) => void
}
export const AppearanceSettingsPageView = ({
  appearance,
  isEntitled,
  updateAppearance,
}: AppearanceSettingsPageViewProps): JSX.Element => {
  const styles = useStyles()
  const theme = useTheme()
  const [t] = useTranslation("appearanceSettings")
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
  return (
    <>
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
