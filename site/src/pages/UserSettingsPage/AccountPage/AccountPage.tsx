import { FC, useState } from "react"
import { Section } from "../../../components/SettingsLayout/Section"
import { AccountForm } from "../../../components/SettingsAccountForm/SettingsAccountForm"
import { useAuth } from "components/AuthProvider/AuthProvider"
import { useMe } from "hooks/useMe"
import { usePermissions } from "hooks/usePermissions"
import { Dialog } from "components/Dialogs/Dialog"
import TextField from "@mui/material/TextField"
import { FormFields, VerticalForm } from "components/Form/Form"
import { LoadingButton } from "components/LoadingButton/LoadingButton"
import Box from "@mui/material/Box"
import GitHubIcon from "@mui/icons-material/GitHub"
import KeyIcon from "@mui/icons-material/VpnKey"
import Button from "@mui/material/Button"
import { MockAuthMethods } from "testHelpers/entities"
import CircularProgress from "@mui/material/CircularProgress"
import { useLocation } from "react-router-dom"
import { retrieveRedirect } from "utils/redirect"
import Typography from "@mui/material/Typography"

type OIDCState =
  | { status: "closed" }
  | { status: "confirmPassword"; error?: unknown }
  | { status: "confirmingPassword" }
  | { status: "selectOIDCProvider" }
  | { status: "updatingProvider" }

export const AccountPage: FC = () => {
  const [authState, authSend] = useAuth()
  const me = useMe()
  const permissions = usePermissions()
  const { updateProfileError } = authState.context
  const canEditUsers = permissions && permissions.updateUsers
  const [OIDCState, setOIDCState] = useState<OIDCState>({
    status: "closed",
  })
  const location = useLocation()
  const redirectTo = retrieveRedirect(location.search)

  return (
    <>
      <Section title="Account" description="Update your account info">
        <AccountForm
          editable={Boolean(canEditUsers)}
          email={me.email}
          updateProfileError={updateProfileError}
          isLoading={authState.matches("signedIn.profile.updatingProfile")}
          initialValues={{
            username: me.username,
          }}
          onSubmit={(data) => {
            authSend({
              type: "UPDATE_PROFILE",
              data,
            })
          }}
          onChangeToOIDCAuth={() => {
            setOIDCState({ status: "confirmPassword" })
          }}
        />
      </Section>
      <OIDCChangeModal
        redirectTo={redirectTo}
        state={OIDCState}
        onChangeState={setOIDCState}
      />
    </>
  )
}

const OIDCChangeModal = ({
  state,
  onChangeState,
  redirectTo,
}: {
  redirectTo: string
  state: OIDCState
  onChangeState: (newState: OIDCState) => void
}) => {
  const authMethods = MockAuthMethods

  const updateProvider = (provider: string) => {
    onChangeState({ status: "updatingProvider" })
    setTimeout(() => {
      window.location.href = `/api/v2/users/oidc/callback?oidc_merge_state=something&redirect=${encodeURIComponent(
        redirectTo,
      )}`
    }, 1000)
  }

  return (
    <Dialog
      open={state.status !== "closed"}
      onClose={() => onChangeState({ status: "closed" })}
      sx={{
        "& .MuiPaper-root": {
          padding: (theme) => theme.spacing(5),
          backgroundColor: (theme) => theme.palette.background.paper,
          border: (theme) => `1px solid ${theme.palette.divider}`,
          width: 440,
        },
      }}
    >
      {(state.status === "confirmPassword" ||
        state.status === "confirmingPassword") && (
        <div>
          <Typography component="h3" sx={{ fontSize: 20 }}>
            Confirm password
          </Typography>
          <Typography
            component="p"
            sx={{
              color: (theme) => theme.palette.text.secondary,
              mt: 1,
              mb: 3,
            }}
          >
            We need to confirm your identity in order to proceed with the
            authentication changes
          </Typography>
          <VerticalForm
            onSubmit={async (e) => {
              e.preventDefault()
              onChangeState({ status: "confirmingPassword" })
              await new Promise((resolve) => setTimeout(resolve, 1000))
              onChangeState({ status: "selectOIDCProvider" })
            }}
          >
            <FormFields>
              <TextField
                type="password"
                label="Password"
                name="password"
                autoFocus
                required
              />
              <LoadingButton
                size="large"
                type="submit"
                variant="contained"
                loading={state.status === "confirmingPassword"}
              >
                Confirm password
              </LoadingButton>
            </FormFields>
          </VerticalForm>
        </div>
      )}

      {(state.status === "selectOIDCProvider" ||
        state.status === "updatingProvider") && (
        <div>
          <Typography component="h3" sx={{ fontSize: 20 }}>
            Select a provider
          </Typography>
          <Typography
            component="p"
            sx={{
              color: (theme) => theme.palette.text.secondary,
              mt: 1,
              mb: 3,
            }}
          >
            After selecting the provider, you will be redirected to the
            provider&lsquo;s authentication page.
          </Typography>
          <Box display="grid" gap="16px">
            <Button
              disabled={state.status === "updatingProvider"}
              onClick={() => updateProvider("github")}
              startIcon={<GitHubIcon sx={{ width: 16, height: 16 }} />}
              fullWidth
              type="submit"
              size="large"
            >
              GitHub
            </Button>
            <Button
              disabled={state.status === "updatingProvider"}
              onClick={() => updateProvider("oidc")}
              size="large"
              startIcon={
                authMethods.oidc.iconUrl ? (
                  <Box
                    component="img"
                    alt="Open ID Connect icon"
                    src={authMethods.oidc.iconUrl}
                    sx={{ width: 16, height: 16 }}
                  />
                ) : (
                  <KeyIcon sx={{ width: 16, height: 16 }} />
                )
              }
              fullWidth
              type="submit"
            >
              {authMethods.oidc.signInText || "OpenID Connect"}
            </Button>
          </Box>
          {state.status === "updatingProvider" && (
            <Box
              sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                mt: (theme) => theme.spacing(2),
                gap: 1,
                fontSize: 13,
                color: (theme) => theme.palette.text.secondary,
              }}
            >
              <CircularProgress size={12} />
              Updating authentication method...
            </Box>
          )}
        </div>
      )}
    </Dialog>
  )
}

export default AccountPage
