import InfoOutlined from "@mui/icons-material/InfoOutlined";
import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import Snackbar from "@mui/material/Snackbar";
import { type FC, type HTMLAttributes, Suspense } from "react";
import { Outlet } from "react-router-dom";
import { Loader } from "components/Loader/Loader";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { LicenseBanner } from "modules/dashboard/LicenseBanner/LicenseBanner";
import { ServiceBanner } from "modules/dashboard/ServiceBanner/ServiceBanner";
import { dashboardContentBottomPadding } from "theme/constants";
import { docs } from "utils/docs";
import { DeploymentBanner } from "./DeploymentBanner/DeploymentBanner";
import { Navbar } from "./Navbar/Navbar";
import { useUpdateCheck } from "./useUpdateCheck";

export const DashboardLayout: FC = () => {
  const { permissions } = useAuthenticated();
  const updateCheck = useUpdateCheck(permissions.viewUpdateCheck);
  const canViewDeployment = Boolean(permissions.viewDeploymentValues);

  return (
    <>
      <ServiceBanner />
      {canViewDeployment && <LicenseBanner />}

      <div
        css={{
          display: "flex",
          minHeight: "100%",
          flexDirection: "column",
        }}
      >
        <Navbar />

        <div
          css={{
            flex: 1,
            paddingBottom: dashboardContentBottomPadding, // Add bottom space since we don't use a footer
            display: "flex",
            flexDirection: "column",
          }}
        >
          <Suspense fallback={<Loader />}>
            <Outlet />
          </Suspense>
        </div>

        <DeploymentBanner />

        <Snackbar
          data-testid="update-check-snackbar"
          open={updateCheck.isVisible}
          anchorOrigin={{
            vertical: "bottom",
            horizontal: "right",
          }}
          ContentProps={{
            sx: (theme) => ({
              background: theme.palette.background.paper,
              color: theme.palette.text.primary,
              maxWidth: 440,
              flexDirection: "row",
              borderColor: theme.palette.info.light,

              "& .MuiSnackbarContent-message": {
                flex: 1,
              },

              "& .MuiSnackbarContent-action": {
                marginRight: 0,
              },
            }),
          }}
          message={
            <div css={{ display: "flex", gap: 16 }}>
              <InfoOutlined
                css={(theme) => ({
                  fontSize: 16,
                  height: 20, // 20 is the height of the text line so we can align them
                  color: theme.palette.info.light,
                })}
              />
              <p>
                Coder {updateCheck.data?.version} is now available. View the{" "}
                <Link href={updateCheck.data?.url}>release notes</Link> and{" "}
                <Link href={docs("/admin/upgrade")}>upgrade instructions</Link>{" "}
                for more information.
              </p>
            </div>
          }
          action={
            <Button variant="text" size="small" onClick={updateCheck.dismiss}>
              Dismiss
            </Button>
          }
        />
      </div>
    </>
  );
};

export const DashboardFullPage: FC<HTMLAttributes<HTMLDivElement>> = ({
  children,
  ...attrs
}) => {
  return (
    <div
      {...attrs}
      css={{
        marginBottom: `-${dashboardContentBottomPadding}px`,
        flex: 1,
        display: "flex",
        flexDirection: "column",
        flexBasis: 0,
        minHeight: "100%",
      }}
    >
      {children}
    </div>
  );
};
