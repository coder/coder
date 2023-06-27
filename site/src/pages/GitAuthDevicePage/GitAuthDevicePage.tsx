import OpenInNewIcon from "@mui/icons-material/OpenInNew"
import CircularProgress from "@mui/material/CircularProgress"
import Link from "@mui/material/Link"
import { makeStyles } from "@mui/styles"
import { useQuery } from "@tanstack/react-query"
import { exchangeGitAuthDevice } from "api/api"
import { isAxiosError } from "axios"
import { Alert } from "components/Alert/Alert"
import { CopyButton } from "components/CopyButton/CopyButton"
import { SignInLayout } from "components/SignInLayout/SignInLayout"
import { Welcome } from "components/Welcome/Welcome"
import { FC, useEffect, useState } from "react"
import { useNavigate, useParams, useSearchParams } from "react-router-dom"

const GitAuthDevicePage: FC = () => {
  const styles = useStyles()
  const { provider } = useParams()
  const [searchParams, setSearchParams] = useSearchParams()
  const navigate = useNavigate()
  const [deviceCode] = useState(() => searchParams.get("device_code") || "")
  const [userCode] = useState(() => searchParams.get("user_code") || "")
  const [verificationUri] = useState(
    () => searchParams.get("verification_uri") || "",
  )
  const [interval] = useState(
    () => Number.parseInt(searchParams.get("interval") || "") || 5,
  )

  useEffect(() => {
    // This will clear the query parameters. It's a nice UX, because
    // then a user can reload the page to obtain a new device code in
    // case the current one expires!
    if (deviceCode) {
      setSearchParams({})
    }
  }, [deviceCode, setSearchParams, navigate])

  const exchange = useQuery({
    queryFn: () =>
      exchangeGitAuthDevice(provider as string, { device_code: deviceCode }),
    queryKey: ["gitauth", provider as string, deviceCode],
    retry: true,
    retryDelay: interval * 1000,
    refetchOnWindowFocus: (query) =>
      query.state.status === "success" ? false : "always",
  })

  useEffect(() => {
    if (!exchange.isSuccess) {
      return
    }
    // Redirect to `/gitauth` to notify any listeners that the
    // authentication was successful.
    navigate("/gitauth")
  }, [exchange.isSuccess, navigate])

  let status = (
    <p className={styles.status}>
      <CircularProgress size={16} color="secondary" />
      Checking for authentication...
    </p>
  )
  if (isAxiosError(exchange.failureReason)) {
    // See https://datatracker.ietf.org/doc/html/rfc8628#section-3.5
    switch (exchange.failureReason.response?.data?.detail) {
      case "authorization_pending":
        break
      case "expired_token":
        status = (
          <Alert severity="error">
            The one-time code has expired. Refresh to get a new one!
          </Alert>
        )
        break
      case "access_denied":
        status = (
          <Alert severity="error">Access to the Git provider was denied.</Alert>
        )
        break
    }
  }

  return (
    <SignInLayout>
      <Welcome message="Authenticate with Git" />

      <p className={styles.text}>
        Copy your one-time code:&nbsp;
        <div className={styles.copyCode}>
          <span className={styles.code}>{userCode}</span>&nbsp;{" "}
          <CopyButton text={userCode} />
        </div>
        <br />
        Then open the link below and paste it:
      </p>
      <Link
        className={styles.link}
        href={verificationUri}
        target="_blank"
        rel="noreferrer"
      >
        <OpenInNewIcon fontSize="small" />
        Open and Paste
      </Link>
      {status}
    </SignInLayout>
  )
}

export default GitAuthDevicePage

const useStyles = makeStyles((theme) => ({
  title: {
    fontSize: theme.spacing(4),
    fontWeight: 400,
    lineHeight: "140%",
    margin: 0,
  },

  text: {
    fontSize: 16,
    color: theme.palette.text.secondary,
    marginBottom: 0,
    textAlign: "center",
    lineHeight: "160%",
  },

  copyCode: {
    display: "inline-flex",
    alignItems: "center",
  },

  code: {
    fontWeight: "bold",
    color: theme.palette.text.primary,
  },

  lineBreak: {
    whiteSpace: "nowrap",
  },

  link: {
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    fontSize: 16,
    gap: theme.spacing(1),
    margin: theme.spacing(2, 0),
  },

  status: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(1),
    color: theme.palette.text.disabled,
  },
}))
