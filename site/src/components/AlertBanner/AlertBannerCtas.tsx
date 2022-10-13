import { FC } from "react"
import { AlertBannerProps } from "./alertTypes"
import { Stack } from "components/Stack/Stack"
import Button from "@material-ui/core/Button"
import RefreshIcon from "@material-ui/icons/Refresh"
import { useTranslation } from "react-i18next"

type AlertBannerCtasProps = Pick<
  AlertBannerProps,
  "actions" | "dismissible" | "retry"
> & {
  setOpen: (arg0: boolean) => void
}

export const AlertBannerCtas: FC<AlertBannerCtasProps> = ({
  actions = [],
  dismissible,
  retry,
  setOpen,
}) => {
  const { t } = useTranslation("common")

  return (
    <Stack direction="row">
      {/* CTAs passed in by the consumer */}
      {actions.length > 0 &&
        actions.map((action) => <div key={String(action)}>{action}</div>)}

      {/* retry CTA */}
      {retry && (
        <div>
          <Button
            size="small"
            onClick={retry}
            startIcon={<RefreshIcon />}
            variant="outlined"
          >
            {t("ctas.retry")}
          </Button>
        </div>
      )}

      {/* close CTA */}
      {dismissible && (
        <Button size="small" onClick={() => setOpen(false)} variant="outlined">
          {t("ctas.dismissCta")}
        </Button>
      )}
    </Stack>
  )
}
