import Button from "@mui/material/Button"
import Link from "@mui/material/Link"
import ArrowRightAltOutlined from "@mui/icons-material/ArrowRightAltOutlined"
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
            href="https://coder.com/docs/coder-oss/latest/admin/upgrade"
            target="_blank"
            rel="noreferrer"
          >
            <Button size="small" startIcon={<ArrowRightAltOutlined />}>
              {t("paywall.actions.upgrade")}
            </Button>
          </Link>
          <Link
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
