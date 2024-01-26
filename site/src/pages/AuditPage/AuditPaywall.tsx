import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import ArrowRightAltOutlined from "@mui/icons-material/ArrowRightAltOutlined";
import { Paywall } from "components/Paywall/Paywall";
import { Stack } from "components/Stack/Stack";
import { FC } from "react";
import { docs } from "utils/docs";

export const AuditPaywall: FC = () => {
  return (
    <Paywall
      message="Audit logs"
      description="Audit Logs allows Auditors to monitor user operations in their deployment. To use this feature, you need an Enterprise license."
      cta={
        <Stack direction="row" alignItems="center">
          <Link target="_blank" rel="noreferrer">
            <Button
              href={docs("/admin/upgrade")}
              size="small"
              startIcon={<ArrowRightAltOutlined />}
            >
              See how to upgrade
            </Button>
          </Link>
          <Link
            href={docs("/admin/audit-logs")}
            target="_blank"
            rel="noreferrer"
          >
            Read the documentation
          </Link>
        </Stack>
      }
    />
  );
};
