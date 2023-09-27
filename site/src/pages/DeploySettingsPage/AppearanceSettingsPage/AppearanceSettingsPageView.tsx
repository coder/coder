import { useState } from "react";
import { Header } from "components/DeploySettingsLayout/Header";
import {
  Badges,
  DisabledBadge,
  EnterpriseBadge,
  EntitledBadge,
} from "components/DeploySettingsLayout/Badges";
import InputAdornment from "@mui/material/InputAdornment";
import { Fieldset } from "components/DeploySettingsLayout/Fieldset";
import { getFormHelpers } from "utils/formUtils";
import Button from "@mui/material/Button";
import FormControlLabel from "@mui/material/FormControlLabel";
import { BlockPicker } from "react-color";
import makeStyles from "@mui/styles/makeStyles";
import Switch from "@mui/material/Switch";
import TextField from "@mui/material/TextField";
import { UpdateAppearanceConfig } from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import { useFormik } from "formik";
import { useTheme } from "@mui/styles";
import Link from "@mui/material/Link";
import { colors } from "theme/colors";
import { hslToHex } from "utils/colors";

export type AppearanceSettingsPageViewProps = {
  appearance: UpdateAppearanceConfig;
  isEntitled: boolean;
  onSaveAppearance: (
    newConfig: Partial<UpdateAppearanceConfig>,
    preview: boolean,
  ) => void;
};

const fallbackBgColor = hslToHex(colors.blue[7]);

export const AppearanceSettingsPageView = ({
  appearance,
  isEntitled,
  onSaveAppearance,
}: AppearanceSettingsPageViewProps): JSX.Element => {
  const styles = useStyles();
  const theme = useTheme();

  const logoForm = useFormik<{
    logo_url: string;
  }>({
    initialValues: {
      logo_url: appearance.logo_url,
    },
    onSubmit: (values) => onSaveAppearance(values, false),
  });
  const logoFieldHelpers = getFormHelpers(logoForm);

  const serviceBannerForm = useFormik<UpdateAppearanceConfig["service_banner"]>(
    {
      initialValues: {
        message: appearance.service_banner.message,
        enabled: appearance.service_banner.enabled,
        background_color:
          appearance.service_banner.background_color ?? fallbackBgColor,
      },
      onSubmit: (values) =>
        onSaveAppearance(
          {
            service_banner: values,
          },
          false,
        ),
    },
  );
  const serviceBannerFieldHelpers = getFormHelpers(serviceBannerForm);

  const [backgroundColor, setBackgroundColor] = useState(
    serviceBannerForm.values.background_color,
  );

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
        subtitle="Specify a custom URL for your logo to be displayed in the top left
          corner of the dashboard."
        validation={
          isEntitled
            ? "We recommend a transparent image with 3:1 aspect ratio."
            : "This is an Enterprise only feature."
        }
        onSubmit={logoForm.handleSubmit}
        button={!isEntitled && <Button disabled>Submit</Button>}
      >
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
        subtitle="Configure a banner that displays a message to all users."
        onSubmit={serviceBannerForm.handleSubmit}
        button={
          !isEntitled && (
            <Button
              onClick={() => {
                onSaveAppearance(
                  {
                    service_banner: {
                      message:
                        "👋 **This** is a service banner. The banner's color and text are editable.",
                      background_color: "#004852",
                      enabled: true,
                    },
                  },
                  true,
                );
              }}
            >
              Show Preview
            </Button>
          )
        }
        validation={
          !isEntitled && (
            <p>
              Your license does not include Service Banners.{" "}
              <Link href="mailto:sales@coder.com">Contact sales</Link> to learn
              more.
            </p>
          )
        }
      >
        {isEntitled && (
          <Stack>
            <FormControlLabel
              control={
                <Switch
                  checked={serviceBannerForm.values.enabled}
                  onChange={async () => {
                    const newState = !serviceBannerForm.values.enabled;
                    const newBanner = {
                      ...serviceBannerForm.values,
                      enabled: newState,
                    };
                    onSaveAppearance(
                      {
                        service_banner: newBanner,
                      },
                      false,
                    );
                    await serviceBannerForm.setFieldValue("enabled", newState);
                  }}
                />
              }
              label="Enabled"
            />
            <Stack spacing={0}>
              <TextField
                {...serviceBannerFieldHelpers(
                  "message",
                  "Markdown bold, italics, and links are supported.",
                )}
                fullWidth
                label="Message"
                multiline
              />
            </Stack>

            <Stack spacing={0}>
              <h3>{"Background Color"}</h3>
              <BlockPicker
                color={backgroundColor}
                onChange={async (color) => {
                  setBackgroundColor(color.hex);
                  await serviceBannerForm.setFieldValue(
                    "background_color",
                    color.hex,
                  );
                  onSaveAppearance(
                    {
                      service_banner: {
                        ...serviceBannerForm.values,
                        background_color: color.hex,
                      },
                    },
                    true,
                  );
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
  );
};

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
}));
