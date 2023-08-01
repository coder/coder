import { Alert } from "components/Alert/Alert"
import { Link as RouterLink } from "react-router-dom"
import Link from "@mui/material/Link"
import { colors } from "theme/colors"

export const HealthBanner = () => {
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
      We detected issues with your Coder deployment. Please,{" "}
      <Link
        component={RouterLink}
        to="/health"
        sx={{ fontWeight: 600, color: "inherit" }}
      >
        check the health status
      </Link>
      .
    </Alert>
  )
}
