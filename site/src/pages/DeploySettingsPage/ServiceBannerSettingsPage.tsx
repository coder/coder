import TextField from "@material-ui/core/TextField"
import { useActor } from "@xstate/react"
import { FeatureNames } from "api/types"
import {
  Badges,
  DisabledBadge,
  EnabledBadge,
  EnterpriseBadge,
  EntitledBadge,
} from "components/DeploySettingsLayout/Badges"
import { Header } from "components/DeploySettingsLayout/Header"
import { LoadingButton } from "components/LoadingButton/LoadingButton"
import { Stack } from "components/Stack/Stack"
import { FormikContextType, useFormik } from "formik"
import React, { useContext } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import * as Yup from "yup"
import { XServiceContext } from "xServices/StateContext"
import { getFormHelpers } from "util/formUtils"
import makeStyles from "@material-ui/core/styles/makeStyles"
import FormControlLabel from "@material-ui/core/FormControlLabel"
import Switch from "@material-ui/core/Switch"

export const Language = {
  messageLabel: "Message",
  backgroundColorLabel: "Background Color",
  updateBanner: "Update",
}

export interface ServiceBannerFormValues {
  message?: string
  backgroundColor?: string
  enabled?: boolean
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

  const onSubmit = (values: ServiceBannerFormValues) => {
    const newBanner = {
      ...serviceBanner,
    }
    if (values.message !== undefined) {
      newBanner.message = values.message
    }
    if (values.enabled !== undefined) {
      newBanner.enabled = values.enabled
    }
    if (values.backgroundColor !== undefined) {
      newBanner.background_color = values.backgroundColor
    }

    serviceBannerSend({
      type: "SET_PREVIEW",
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
      onSubmit,
    })
  const getFieldHelpers = getFormHelpers<ServiceBannerFormValues>(form)

  return (
    <>
      <Helmet>
        <title>{pageTitle("Service Banner Settings")}</title>
      </Helmet>

      <Header
        title="Service Banner"
        description="Configure the Service Banner"
        docsHref="https://coder.com/docs/coder-oss/latest/admin/high-availability#service-banners"
      />
      <Badges>
        {isEntitled ? <EntitledBadge /> : <DisabledBadge />}
        <EnterpriseBadge />
      </Badges>

      <form
        className={styles.form}
        onSubmit={form.handleSubmit}
        // onChange={form.handleSubmit}
      >
        <Stack>
          <FormControlLabel
            value="enable"
            control={<Switch {...getFieldHelpers("enabled")} color="primary" />}
            label="Enable"
          />
          <TextField
            fullWidth
            {...getFieldHelpers("message")}
            label={Language.messageLabel}
            variant="outlined"
          />

          <LoadingButton
            loading={false}
            //   aria-disabled={!editable}
            //   disabled={!editable}
            type="submit"
            variant="contained"
          >
            {Language.updateBanner}
          </LoadingButton>
        </Stack>
      </form>
    </>
  )
}
const useStyles = makeStyles(() => ({
  form: {
    maxWidth: "500px",
  },
}))

export default ServiceBannerSettingsPage
