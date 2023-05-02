import Button from "@material-ui/core/Button"
import { makeStyles, useTheme } from "@material-ui/core/styles"
import Skeleton from "@material-ui/lab/Skeleton"
import { GetLicensesResponse } from "api/api"
import { Header } from "components/DeploySettingsLayout/Header"
import { LicenseCard } from "components/LicenseCard/LicenseCard"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import Confetti from "react-confetti"
import { Link } from "react-router-dom"
import useWindowSize from "react-use/lib/useWindowSize"

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
        width={width}
        height={height}
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
          description="Enterprise licenses unlock more features on your deployment."
        />

        <Button
          variant="outlined"
          component={Link}
          to="/settings/deployment/licenses/add"
        >
          Add new license
        </Button>
      </Stack>

      {isLoading && <Skeleton variant="rect" height={200} />}

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
              <span className={styles.title}>No licenses yet</span>
              <span className={styles.description}>
                Contact <a href="mailto:sales@coder.com">sales</a> or{" "}
                <a href="https://coder.com/trial">request a trial license</a> to
                learn more.
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

    "&:hover": {
      backgroundColor: theme.palette.background.paper,
    },
  },

  description: {
    color: theme.palette.text.secondary,
    textAlign: "center",
    maxWidth: theme.spacing(50),
  },
}))

export default LicensesSettingsPageView
