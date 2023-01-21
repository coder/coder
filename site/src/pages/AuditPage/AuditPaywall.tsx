import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import ArrowRightAltOutlined from "@material-ui/icons/ArrowRightAltOutlined"
import { Paywall } from "components/Paywall/Paywall"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import { useTranslation } from "react-i18next"

export const AuditPaywall: FC = () => {
  const { t } = useTranslation("auditLog")

  return (
    <Paywall
      message={t("paywall.title")}
      description={t("paywall.description")}
      cta={
        <Stack direction="row" alignItems="center">
          <Link
            underline="none"
            href="https://coder.com/docs/coder-oss/latest/admin/upgrade"
            target="_blank"
            rel="noreferrer"
          >
            <Button size="small" startIcon={<ArrowRightAltOutlined />}>
              {t("paywall.actions.upgrade")}
            </Button>
          </Link>
          <Link
            underline="none"
            href="https://coder.com/docs/coder-oss/latest/admin/audit-logs"
            target="_blank"
            rel="noreferrer"
          >
            {t("paywall.actions.readDocs")}
          </Link>
        </Stack>
      }
    />
  )
}
