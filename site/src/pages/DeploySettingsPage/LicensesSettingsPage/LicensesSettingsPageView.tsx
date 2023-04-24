import Button from "@material-ui/core/Button"
import { makeStyles, useTheme } from "@material-ui/core/styles"
import Skeleton from "@material-ui/lab/Skeleton"
import { GetLicensesResponse, removeLicense } from "api/api"
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
          Add new License
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

      {!isLoading && licenses && licenses.length === 0 && (
        <Stack spacing={4} justifyContent="center" alignItems="center">
          <Button className={styles.ctaButton} size="large">
            Add license
          </Button>
        </Stack>
      )}
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  ctaButton: {
    backgroundImage: `linear-gradient(90deg, ${theme.palette.secondary.main} 0%, ${theme.palette.secondary.dark} 100%)`,
    width: theme.spacing(30),
    marginBottom: theme.spacing(4),
    marginTop: theme.spacing(4),
  },
}))

export default LicensesSettingsPageView
