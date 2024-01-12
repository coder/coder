import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import Button from "@mui/material/Button";
import Skeleton from "@mui/material/Skeleton";
import AddIcon from "@mui/icons-material/AddOutlined";
import RefreshIcon from "@mui/icons-material/Refresh";
import MuiLink from "@mui/material/Link";
import Tooltip from "@mui/material/Tooltip";
import LoadingButton from "@mui/lab/LoadingButton";
import { type FC } from "react";
import Confetti from "react-confetti";
import { Link } from "react-router-dom";
import useWindowSize from "react-use/lib/useWindowSize";
import type { GetLicensesResponse } from "api/api";
import { Stack } from "components/Stack/Stack";
import { LicenseCard } from "./LicenseCard";
import { Header } from "../Header";

type Props = {
  showConfetti: boolean;
  isLoading: boolean;
  userLimitActual?: number;
  userLimitLimit?: number;
  licenses?: GetLicensesResponse[];
  isRemovingLicense: boolean;
  isRefreshing: boolean;
  removeLicense: (licenseId: number) => void;
  refreshEntitlements: () => void;
};

const LicensesSettingsPageView: FC<Props> = ({
  showConfetti,
  isLoading,
  userLimitActual,
  userLimitLimit,
  licenses,
  isRemovingLicense,
  isRefreshing,
  removeLicense,
  refreshEntitlements,
}) => {
  const theme = useTheme();
  const { width, height } = useWindowSize();

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

        <Stack direction="row" spacing={2}>
          <Button
            component={Link}
            to="/deployment/licenses/add"
            startIcon={<AddIcon />}
          >
            Add a license
          </Button>
          <Tooltip title="Refresh license entitlements. This is done automatically every 10 minutes.">
            <LoadingButton
              loadingPosition="start"
              loading={isRefreshing}
              onClick={refreshEntitlements}
              startIcon={<RefreshIcon />}
            >
              Refresh
            </LoadingButton>
          </Tooltip>
        </Stack>
      </Stack>

      {isLoading && <Skeleton variant="rectangular" height={200} />}

      {!isLoading && licenses && licenses?.length > 0 && (
        <Stack spacing={4}>
          {licenses
            ?.sort(
              (a, b) =>
                new Date(b.claims.license_expires).valueOf() -
                new Date(a.claims.license_expires).valueOf(),
            )
            .map((license) => (
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
        <div css={styles.root}>
          <Stack alignItems="center" spacing={1}>
            <Stack alignItems="center" spacing={0.5}>
              <span css={styles.title}>You don&apos;t have any licenses!</span>
              <span css={styles.description}>
                You&apos;re missing out on high availability, RBAC, quotas, and
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
  );
};

const styles = {
  title: {
    fontSize: 16,
  },

  root: (theme) => ({
    minHeight: 240,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    borderRadius: 8,
    border: `1px solid ${theme.palette.divider}`,
    padding: 48,
  }),

  description: (theme) => ({
    color: theme.palette.text.secondary,
    textAlign: "center",
    maxWidth: 464,
    marginTop: 8,
  }),
} satisfies Record<string, Interpolation<Theme>>;

export default LicensesSettingsPageView;
