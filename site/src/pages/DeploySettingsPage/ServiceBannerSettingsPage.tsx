import { makeStyles } from "@material-ui/core"
import TextField from "@material-ui/core/TextField"
import { useActor } from "@xstate/react"
import { FeatureNames } from "api/types"
import {
  Badges,
  DisabledBadge,
  EnabledBadge,
  EnterpriseBadge,
} from "components/DeploySettingsLayout/Badges"
import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout"
import { Header } from "components/DeploySettingsLayout/Header"
import OptionsTable from "components/DeploySettingsLayout/OptionsTable"
import { LoadingButton } from "components/LoadingButton/LoadingButton"
import { Section } from "components/Section/Section"
import { Stack } from "components/Stack/Stack"
import { FormikContextType, useFormik } from "formik"
import React, { useContext, useEffect } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"
import * as Yup from "yup"
import { XServiceContext } from "xServices/StateContext"
import { ServiceBanner } from "api/typesGenerated"
import { getFormHelpers } from "util/formUtils"

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

const ServiceBannerSettingsPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [serviceBannerState, serviceBannerSend] = useActor(
    xServices.serviceBannerXService,
  )

  const [entitlementsState] = useActor(xServices.entitlementsXService)

  const serviceBanner = serviceBannerState.context.serviceBanner

  /** Gets license data on app mount because LicenseBanner is mounted in App */
  useEffect(() => {
    serviceBannerSend("GET_BANNER")
  }, [serviceBannerSend])

  const styles = useStyles()

  const isEntitled =
    entitlementsState.context.entitlements.features[
      FeatureNames.HighAvailability
    ].entitlement !== "not_entitled"

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
        {isEntitled ? <EnabledBadge /> : <DisabledBadge />}
        <EnterpriseBadge />
      </Badges>

      <form className={styles.form} onSubmit={form.handleSubmit}>
        <Stack>
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
