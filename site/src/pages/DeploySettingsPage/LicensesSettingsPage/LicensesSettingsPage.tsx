import Button from "@material-ui/core/Button"
import { makeStyles, useTheme } from "@material-ui/core/styles"
import Skeleton from "@material-ui/lab/Skeleton"
import { useMutation, useQuery } from "@tanstack/react-query"
import { useMachine } from "@xstate/react"
import { getLicenses, removeLicense } from "api/api"
import { Header } from "components/DeploySettingsLayout/Header"
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils"
import { LicenseCard } from "components/LicenseCard/LicenseCard"
import { Stack } from "components/Stack/Stack"
import { FC, useEffect } from "react"
import Confetti from "react-confetti"
import { Helmet } from "react-helmet-async"
import { Link, useSearchParams } from "react-router-dom"
import useToggle from "react-use/lib/useToggle"
import useWindowSize from "react-use/lib/useWindowSize"
import { pageTitle } from "utils/page"
import { entitlementsMachine } from "xServices/entitlements/entitlementsXService"

const LicensesSettingsPage: FC = () => {
  const [entitlementsState] = useMachine(entitlementsMachine)
  const { entitlements } = entitlementsState.context
  const styles = useStyles()
  const [searchParams, setSearchParams] = useSearchParams()
  const success = searchParams.get("success")
  const [confettiOn, toggleConfettiOn] = useToggle(false)
  const { width, height } = useWindowSize()

  const { mutate: removeLicenseApi, isLoading: isRemovingLicense } =
    useMutation(removeLicense)

  const theme = useTheme()

  const {
    data: licenses,
    isLoading,
    refetch: refetchGetLicenses,
  } = useQuery({
    queryKey: ["licenses"],
    queryFn: () => getLicenses(),
  })

  useEffect(() => {
    if (success) {
      toggleConfettiOn()
      setTimeout(() => {
        toggleConfettiOn(false)
        setSearchParams()
      }, 2000)
    }
  }, [setSearchParams, success, toggleConfettiOn])

  return (
    <>
      <Helmet>
        <title>{pageTitle("License Settings")}</title>
      </Helmet>
      <Confetti
        width={width}
        height={height}
        numberOfPieces={confettiOn ? 200 : 0}
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
              userLimitActual={entitlements?.features.user_limit.actual}
              userLimitLimit={entitlements?.features.user_limit.limit}
              isRemoving={isRemovingLicense}
              onRemove={(licenseId: number) =>
                removeLicenseApi(licenseId, {
                  onSuccess: () => {
                    displaySuccess("Successfully removed license")
                    void refetchGetLicenses()
                  },
                  onError: () => {
                    displayError("Failed to remove license")
                    void refetchGetLicenses()
                  },
                })
              }
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
  },
  removeButtom: {
    color: "red",
  },
}))

export default LicensesSettingsPage
