import Button from "@mui/material/Button"
import { makeStyles, useTheme } from "@mui/styles"
import Skeleton from "@mui/material/Skeleton"
import AddIcon from "@mui/icons-material/AddOutlined"
import { GetLicensesResponse } from "api/api"
import { Header } from "components/DeploySettingsLayout/Header"
import { LicenseCard } from "components/LicenseCard/LicenseCard"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import Confetti from "react-confetti"
import { Link } from "react-router-dom"
import useWindowSize from "react-use/lib/useWindowSize"
import MuiLink from "@mui/material/Link"

type Props = {
  showConfetti: boolean
  isLoading: boolean
  userLimitActual?: number
  userLimitLimit?: number
  licenses?: GetLicensesResponse[]
  isRemovingLicense: boolean
  removeLicense: (licenseId: number) => void
}

const LicensesSettingsPageView: FC<Props> = ({
  showConfetti,
  isLoading,
  userLimitActual,
  userLimitLimit,
  licenses,
  isRemovingLicense,
  removeLicense,
}) => {
  const styles = useStyles()
  const { width, height } = useWindowSize()

  const theme = useTheme()

  return (
    <>
      <Confetti
        // For some reason this overflows the window and adds scrollbars if we don't subtract here.
        width={width - 1}
        height={height - 1}
        numberOfPieces={showConfetti ? 200 : 0}
        colors={[theme.palette.primary.main, theme.palette.secondary.main]}
      />
      <Stack
        alignItems="baseline"
        direction="row"
        justifyContent="space-between"
      >
        <Header
          title="Licenses"
          description="Manage licenses to unlock Enterprise features."
        />

        <Button
          component={Link}
          to="/settings/deployment/licenses/add"
          startIcon={<AddIcon />}
        >
          Add a license
        </Button>
      </Stack>

      {isLoading && <Skeleton variant="rectangular" height={200} />}

      {!isLoading && licenses && licenses?.length > 0 && (
        <Stack spacing={4}>
          {licenses?.map((license) => (
            <LicenseCard
              key={license.id}
              license={license}
              userLimitActual={userLimitActual}
              userLimitLimit={userLimitLimit}
              isRemoving={isRemovingLicense}
              onRemove={removeLicense}
            />
          ))}
        </Stack>
      )}

      {!isLoading && licenses === null && (
        <div className={styles.root}>
          <Stack alignItems="center" spacing={1}>
            <Stack alignItems="center" spacing={0.5}>
              <span className={styles.title}>
                You don{"'"}t have any licenses!
              </span>
              <span className={styles.description}>
                You{"'"}re missing out on high availability, RBAC, quotas, and
                much more. Contact{" "}
                <MuiLink href="mailto:sales@coder.com">sales</MuiLink> or{" "}
                <MuiLink href="https://coder.com/trial">
                  request a trial license
                </MuiLink>{" "}
                to get started.
              </span>
            </Stack>
          </Stack>
        </div>
      )}
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  title: {
    fontSize: theme.spacing(2),
  },

  root: {
    minHeight: theme.spacing(30),
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,
    padding: theme.spacing(6),
  },

  description: {
    color: theme.palette.text.secondary,
    textAlign: "center",
    maxWidth: theme.spacing(58),
    marginTop: theme.spacing(1),
  },
}))

export default LicensesSettingsPageView
