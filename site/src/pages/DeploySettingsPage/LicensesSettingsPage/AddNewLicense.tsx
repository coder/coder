
import { useQuery } from "@tanstack/react-query"
import { getLicenses } from "api/api"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "utils/page"
import { License } from "api/typesGenerated"
import { Header } from "components/DeploySettingsLayout/Header"
import { Button, Card, CardContent, Typography } from "@material-ui/core"
import dayjs from "dayjs"
import { PageHeader, PageHeaderSubtitle, PageHeaderTitle } from "components/PageHeader/PageHeader"
import { PlusOneOutlined } from "@material-ui/icons"

const AddNewLicense: FC = () => {

  return (
    <>
      <Helmet>
        <title>{pageTitle("Add new License")}</title>
      </Helmet>
      <Stack alignItems="baseline" direction="row" justifyContent="space-between">
        <Header
          title="Add your license"
          description="Add a license to your account to unlock more features."
        />
      </Stack>

      <Stack spacing={4}>

      </Stack>
    </>
  )
}

export default AddNewLicense
