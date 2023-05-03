import Button from "@material-ui/core/Button"
import Paper from "@material-ui/core/Paper"
import { makeStyles } from "@material-ui/core/styles"
import { License } from "api/typesGenerated"
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import { Stack } from "components/Stack/Stack"
import dayjs from "dayjs"
import { useState } from "react"

type LicenseCardProps = {
  license: License
  userLimitActual?: number
  userLimitLimit?: number
  onRemove: (licenseId: number) => void
  isRemoving: boolean
}

export const LicenseCard = ({
  license,
  userLimitActual,
  userLimitLimit,
  onRemove,
  isRemoving,
}: LicenseCardProps) => {
  const styles = useStyles()

  const [licenseIDMarkedForRemoval, setLicenseIDMarkedForRemoval] = useState<
    number | undefined
  >(undefined)

  return (
    <Paper
      variant="outlined"
      key={license.id}
      elevation={2}
      className={styles.licenseCard}
    >
      <ConfirmDialog
        type="info"
        hideCancel={false}
        open={licenseIDMarkedForRemoval !== undefined}
        onConfirm={() => {
          if (!licenseIDMarkedForRemoval) {
            return
          }
          onRemove(licenseIDMarkedForRemoval)
          setLicenseIDMarkedForRemoval(undefined)
        }}
        onClose={() => setLicenseIDMarkedForRemoval(undefined)}
        title="Confirm License Removal"
        confirmLoading={isRemoving}
        confirmText="Remove"
        description="Removing this license will disable all Enterprise features. You add a new license at any time."
      />
      <Stack
        direction="row"
        spacing={2}
        className={styles.cardContent}
        justifyContent="left"
        alignItems="center"
      >
        <span className={styles.licenseId}>#{license.id}</span>
        <span className={styles.accountType}>
          {license.claims.trial ? "Trial" : "Enterprise"}
        </span>
        <Stack
          direction="row"
          justifyContent="right"
          spacing={8}
          alignItems="self-end"
          style={{
            flex: 1,
          }}
        >
          <Stack direction="column" spacing={0}>
            <span className={styles.secondaryMaincolor}>Users</span>
            <span className={styles.userLimit}>
              {userLimitActual} {` / ${userLimitLimit || "Unlimited"}`}
            </span>
          </Stack>

          <Stack direction="column" spacing={0}>
            <span className={styles.secondaryMaincolor}>Valid Until</span>
            <span className={styles.licenseExpires}>
              {dayjs
                .unix(license.claims.license_expires)
                .format("MMMM D, YYYY")}
            </span>
          </Stack>
          <Button
            className={styles.removeButton}
            variant="text"
            size="small"
            onClick={() => setLicenseIDMarkedForRemoval(license.id)}
          >
            Remove
          </Button>
        </Stack>
      </Stack>
    </Paper>
  )
}

const useStyles = makeStyles((theme) => ({
  userLimit: {
    color: theme.palette.text.primary,
  },
  licenseCard: {
    padding: theme.spacing(2),
  },
  cardContent: {},
  licenseId: {
    color: theme.palette.secondary.main,
    fontSize: 18,
    fontWeight: 600,
  },
  accountType: {
    fontWeight: 600,
    fontSize: 18,
    alignItems: "center",
    textTransform: "capitalize",
  },
  licenseExpires: {
    color: theme.palette.text.secondary,
  },
  secondaryMaincolor: {
    color: theme.palette.text.hint,
  },
  removeButton: {
    height: "17px",
    minHeight: "17px",
    padding: 0,
    border: "none",
    color: theme.palette.error.main,
    "&:hover": {
      backgroundColor: "transparent",
    },
  },
}))
