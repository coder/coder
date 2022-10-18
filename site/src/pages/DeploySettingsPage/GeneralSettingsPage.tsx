import { useDeploySettings } from "components/DeploySettingsLayout/DeploySettingsLayout"
import { Header } from "components/DeploySettingsLayout/Header"
import OptionsTable from "components/DeploySettingsLayout/OptionsTable"
import React from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "util/page"

const GeneralSettingsPage: React.FC = () => {
  const { deploymentFlags } = useDeploySettings()

  return (
    <>
      <Helmet>
        <title>{pageTitle("General Settings")}</title>
      </Helmet>

      <Header
        title="General"
        description="Information about your Coder deployment."
        docsHref="https://coder.com/docs/coder-oss/latest/admin/configure"
      />

      <OptionsTable
        options={{
          access_url: deploymentFlags.access_url,
          address: deploymentFlags.address,
          wildcard_access_url: deploymentFlags.wildcard_access_url,
        }}
      />
    </>
  )
}

export default GeneralSettingsPage
