import { makeStyles } from "@mui/styles"
import { useMachine } from "@xstate/react"
import { DeploymentBanner } from "components/DeploymentBanner/DeploymentBanner"
import { LicenseBanner } from "components/LicenseBanner/LicenseBanner"
import { Loader } from "components/Loader/Loader"
import { ServiceBanner } from "components/ServiceBanner/ServiceBanner"
import { usePermissions } from "hooks/usePermissions"
import { FC, Suspense } from "react"
import { Outlet } from "react-router-dom"
import { dashboardContentBottomPadding } from "theme/constants"
import { updateCheckMachine } from "xServices/updateCheck/updateCheckXService"
import { Navbar } from "../Navbar/Navbar"
import Snackbar from "@mui/material/Snackbar"
import Link from "@mui/material/Link"
import Box from "@mui/material/Box"
import InfoOutlined from "@mui/icons-material/InfoOutlined"
import Button from "@mui/material/Button"

export const DashboardLayout: FC = () => {
  const styles = useStyles()
  const permissions = usePermissions()
  const [updateCheckState, updateCheckSend] = useMachine(updateCheckMachine, {
    context: {
      permissions,
    },
  })
  const { updateCheck } = updateCheckState.context
  const canViewDeployment = Boolean(permissions.viewDeploymentValues)

  return (
    <>
      <ServiceBanner />
      {canViewDeployment && <LicenseBanner />}

      <div className={styles.site}>
        <Navbar />

        <div className={styles.siteContent}>
          <Suspense fallback={<Loader />}>
            <Outlet />
          </Suspense>
        </div>

        <DeploymentBanner />

        <Snackbar
          data-testid="update-check-snackbar"
          open={updateCheckState.matches("show")}
          anchorOrigin={{
            vertical: "bottom",
            horizontal: "right",
          }}
          ContentProps={{
            sx: (theme) => ({
              background: theme.palette.background.paper,
              color: theme.palette.text.primary,
              maxWidth: theme.spacing(55),
              flexDirection: "row",
              borderColor: theme.palette.info.light,

              "& .MuiSnackbarContent-message": {
                flex: 1,
              },

              "& .MuiSnackbarContent-action": {
                marginRight: 0,
              },
            }),
          }}
          message={
            <Box display="flex" gap={2}>
              <InfoOutlined
                sx={(theme) => ({
                  fontSize: 16,
                  height: 20, // 20 is the height of the text line so we can align them
                  color: theme.palette.info.light,
                })}
              />
              <Box>
                Coder {updateCheck?.version} is now available. View the{" "}
                <Link href={updateCheck?.url}>release notes</Link> and{" "}
                <Link href="https://coder.com/docs/coder-oss/latest/admin/upgrade">
                  upgrade instructions
                </Link>{" "}
                for more information.
              </Box>
            </Box>
          }
          action={
            <Button
              variant="text"
              size="small"
              onClick={() => updateCheckSend("DISMISS")}
            >
              Dismiss
            </Button>
          }
        />
      </div>
    </>
  )
}

const useStyles = makeStyles({
  site: {
    display: "flex",
    minHeight: "100vh",
    flexDirection: "column",
  },
  siteContent: {
    flex: 1,
    paddingBottom: dashboardContentBottomPadding, // Add bottom space since we don't use a footer
    display: "flex",
    flexDirection: "column",
  },
})
