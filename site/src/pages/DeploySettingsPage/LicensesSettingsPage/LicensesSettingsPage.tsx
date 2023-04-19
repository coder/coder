import { makeStyles, useTheme } from "@material-ui/core/styles"
import RemoveCircleOutlineSharp from "@material-ui/icons/RemoveCircleOutlineSharp"
import { useQuery } from "@tanstack/react-query"
import { useMachine } from "@xstate/react"
import { getLicenses } from "api/api"
import { Header } from "components/DeploySettingsLayout/Header"
import { Stack } from "components/Stack/Stack"
import dayjs from "dayjs"
import { FC, useEffect } from "react"
import { Helmet } from "react-helmet-async"
import { Link, useSearchParams } from "react-router-dom"
import { pageTitle } from "utils/page"
import { entitlementsMachine } from "xServices/entitlements/entitlementsXService"
import Confetti from "react-confetti"
import useWindowSize from "react-use/lib/useWindowSize"
import useToggle from "react-use/lib/useToggle"
import Button from "@material-ui/core/Button"
import Card from "@material-ui/core/Card"
import CardContent from "@material-ui/core/CardContent"
import Box from "@material-ui/core/Box"
import Skeleton from "@material-ui/lab/Skeleton"

const LicensesSettingsPage: FC = () => {
  const [entitlementsState] = useMachine(entitlementsMachine)
  const { entitlements } = entitlementsState.context
  const styles = useStyles()
  const [searchParams, setSearchParams] = useSearchParams()
  const success = searchParams.get("success")
  const [confettiOn, toggleConfettiOn] = useToggle(false)
  const { width, height } = useWindowSize()

  const theme = useTheme()

  const { data: licenses, isLoading } = useQuery({
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
        <title>{pageTitle("General Settings")}</title>
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
          description="Add a license to your account to unlock more features."
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
            <Card
              variant="outlined"
              key={license.id}
              elevation={2}
              className={styles.licenseCard}
            >
              <CardContent>
                <Stack
                  direction="column"
                  className={styles.cardContent}
                  justifyContent="space-between"
                >
                  <Box className={styles.licenseId}>
                    <span>#{license.id}</span>
                  </Box>
                  <Stack className={styles.accountType}>
                    <span>{license.claims.account_type} License</span>
                  </Stack>
                  <Stack
                    direction="row"
                    justifyContent="space-between"
                    alignItems="center"
                  >
                    <div className={styles.userLimit}>
                      <span className={styles.userLimitActual}>
                        {entitlements?.features.user_limit.actual}
                      </span>
                      <span className={styles.userLimitLimit}>
                        /{" "}
                        {entitlements?.features.user_limit.limit || "Unlimited"}{" "}
                        users
                      </span>
                    </div>

                    <Stack direction="column" spacing={0} alignItems="center">
                      <span className={styles.expirationDate}>
                        {dayjs(license.expires_at).format("MMMM D, YYYY")}
                      </span>
                      <span className={styles.expirationDateLabel}>
                        Valid until
                      </span>
                    </Stack>
                    <div className={styles.actions}>
                      <Button
                        startIcon={<RemoveCircleOutlineSharp />}
                        variant="text"
                        size="small"
                      >
                        Remove
                      </Button>
                    </div>
                  </Stack>
                </Stack>
              </CardContent>
            </Card>
          ))}
        </Stack>
      )}

      {!isLoading && licenses && licenses.length === 0 && (
        <Stack spacing={4} justifyContent="center" alignItems="center">
          <Button className={styles.ctaButton} size="large">
            Add your license
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
  expirationDate: {
    fontWeight: 600,
    color: theme.palette.primary.main,
  },
  expirationDateLabel: {
    color: theme.palette.secondary.main,
    fontSize: theme.typography.subtitle2.fontSize,
  },
  userLimit: {
    width: "33%",
  },
  actions: {
    width: "33%",
    textAlign: "right",
  },
  userLimitActual: {
    // fontWeight: 600,
    fontSize: theme.typography.h3.fontSize,
    paddingRight: theme.spacing(1),
    color: theme.palette.primary.main,
  },
  userLimitLimit: {
    color: theme.palette.secondary.main,
    fontSize: theme.typography.h5.fontSize,
    fontWeight: 600,
  },
  licenseCard: {},
  cardContent: {
    minHeight: 200,
  },
  licenseId: {
    color: theme.palette.secondary.main,
    fontWeight: 600,
    fontSize: theme.typography.h5.fontSize,
  },
  accountType: {
    fontWeight: 600,
    fontSize: theme.typography.pxToRem(32),
    justifyContent: "center",
    alignItems: "center",
    textTransform: "capitalize",
  },
}))

export default LicensesSettingsPage
