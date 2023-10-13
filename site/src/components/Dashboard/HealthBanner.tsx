import { Alert } from "components/Alert/Alert";
import { Link as RouterLink } from "react-router-dom";
import Link from "@mui/material/Link";
import { colors } from "theme/colors";
import { useQuery } from "react-query";
import { getHealth } from "api/api";
import { useDashboard } from "./DashboardProvider";

export const HealthBanner = () => {
  const { data: healthStatus } = useQuery({
    queryKey: ["health"],
    queryFn: () => getHealth(),
  });
  const dashboard = useDashboard();
  const hasHealthIssues = healthStatus && !healthStatus.data.healthy;

  if (
    dashboard.experiments.includes("deployment_health_page") &&
    hasHealthIssues
  ) {
    return (
      <Alert
        severity="error"
        variant="filled"
        sx={{
          border: 0,
          borderRadius: 0,
          backgroundColor: colors.red[10],
        }}
      >
        We have detected problems with your Coder deployment. Please{" "}
        <Link
          component={RouterLink}
          to="/health"
          sx={{ fontWeight: 600, color: "inherit" }}
        >
          inspect the health status
        </Link>
        .
      </Alert>
    );
  }

  return null;
};
