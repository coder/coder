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
import { Link, NavLink } from "react-router-dom"

const LicensesSettingsPage: FC = () => {
  const { data: licenses, isLoading } = useQuery({
    queryKey: ["licenses"],
    queryFn: () => getLicenses()
  })

  console.log(licenses)


  return (
    <>
      <Helmet>
        <title>{pageTitle("General Settings")}</title>
      </Helmet>
      <Stack alignItems="baseline" direction="row" justifyContent="space-between">
        <Header
          title="Licenses"
          description="Add a license to your account to unlock more features."
        />

        <Button
          variant="outlined"
          component={Link}
          to="/settings/deployment/licenses/add"
        >
          Add new license
        </Button>

      </Stack>

      <Stack spacing={4}>
        {licenses?.map((license) => (
          <Card key={license.id}>
            <CardContent>
              <Typography color="textSecondary" variant="h4">
                #{license.id}
              </Typography>
              {/* <p>{dayjs.unix(license.claims.license_expires).toISOString()}</p>
              <p>{license.claims.trial}</p>
              <p>{license.claims.account_type}</p> */}
            </CardContent>
          </Card>
        ))}
      </Stack>
    </>
  )
}

export default LicensesSettingsPage
