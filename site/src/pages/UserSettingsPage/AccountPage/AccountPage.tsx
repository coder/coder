import { ComponentProps, FC, useState } from "react"
import { Section } from "../../../components/SettingsLayout/Section"
import { AccountForm } from "../../../components/SettingsAccountForm/SettingsAccountForm"
import { useAuth } from "components/AuthProvider/AuthProvider"
import { useMe } from "hooks/useMe"
import { usePermissions } from "hooks/usePermissions"
import TextField from "@mui/material/TextField"
import Box from "@mui/material/Box"
import GitHubIcon from "@mui/icons-material/GitHub"
import KeyIcon from "@mui/icons-material/VpnKey"
import Button from "@mui/material/Button"
import { useLocation } from "react-router-dom"
import { retrieveRedirect } from "utils/redirect"
import Typography from "@mui/material/Typography"
import { convertToOAUTH, getAuthMethods } from "api/api"
import { AuthMethods, LoginType } from "api/typesGenerated"
import Skeleton from "@mui/material/Skeleton"
import { Stack } from "components/Stack/Stack"
import { useMutation, useQuery } from "@tanstack/react-query"
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import { getErrorMessage } from "api/errors"

type LoginTypeConfirmation =
  | {
      open: false
      selectedType: undefined
    }
  | {
      open: true
      selectedType: LoginType
    }

export const AccountPage: FC = () => {
  const [authState, authSend] = useAuth()
  const me = useMe()
  const permissions = usePermissions()
  const { updateProfileError } = authState.context
  const canEditUsers = permissions && permissions.updateUsers
  const location = useLocation()
  const redirectTo = retrieveRedirect(location.search)
  const [loginTypeConfirmation, setLoginTypeConfirmation] =
    useState<LoginTypeConfirmation>({ open: false, selectedType: undefined })
  const { data: authMethods } = useQuery({
    queryKey: ["authMethods"],
    queryFn: getAuthMethods,
  })
  const loginTypeMutation = useMutation(convertToOAUTH, {
    onSuccess: (data) => {
      window.location.href = `/api/v2/users/oidc/callback?oidc_merge_state=${
        data.state_string
      }&redirect=${encodeURIComponent(redirectTo)}`
    },
  })

  return (
    <Stack spacing={8}>
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
        />
      </Section>

      <Section
        title="Single Sign On"
        description="Authenticate in Coder using one-click"
      >
        <Box display="grid" gap="16px">
          {authMethods ? (
            authMethods.me_login_type === "password" ? (
              <>
                {authMethods.github.enabled && (
                  <GitHubButton
                    disabled={loginTypeMutation.isLoading}
                    onClick={() =>
                      setLoginTypeConfirmation({
                        open: true,
                        selectedType: "github",
                      })
                    }
                  >
                    GitHub
                  </GitHubButton>
                )}
                {authMethods.oidc.enabled && (
                  <OIDCButton
                    authMethods={authMethods}
                    disabled={loginTypeMutation.isLoading}
                    onClick={() =>
                      setLoginTypeConfirmation({
                        open: true,
                        selectedType: "oidc",
                      })
                    }
                  >
                    {getOIDCLabel(authMethods)}
                  </OIDCButton>
                )}
              </>
            ) : (
              <>
                {authMethods.me_login_type === "github" && (
                  <GitHubButton disabled>
                    Authenticated with GitHub
                  </GitHubButton>
                )}

                {authMethods.me_login_type === "oidc" && (
                  <OIDCButton authMethods={authMethods} disabled>
                    Authenticated with {getOIDCLabel(authMethods)}
                  </OIDCButton>
                )}
              </>
            )
          ) : (
            <>
              <Skeleton
                variant="rectangular"
                sx={{ height: 40, borderRadius: 1 }}
              />
              <Skeleton
                variant="rectangular"
                sx={{ height: 40, borderRadius: 1 }}
              />
            </>
          )}
        </Box>
      </Section>

      <ConfirmLoginTypeChangeModal
        open={loginTypeConfirmation.open}
        error={loginTypeMutation.error}
        // We still want to show it loading when it is success so the modal is
        // not going to close or change until the oauth redirect
        loading={loginTypeMutation.isLoading || loginTypeMutation.isSuccess}
        onClose={() => {
          setLoginTypeConfirmation({ open: false, selectedType: undefined })
          loginTypeMutation.reset()
        }}
        onConfirm={(password) => {
          if (!loginTypeConfirmation.selectedType) {
            throw new Error("No login type selected")
          }
          loginTypeMutation.mutate({
            to_login_type: loginTypeConfirmation.selectedType,
            email: me.email,
            password,
          })
        }}
      />
    </Stack>
  )
}

const GitHubButton = (props: ComponentProps<typeof Button>) => {
  return (
    <Button
      startIcon={<GitHubIcon sx={{ width: 16, height: 16 }} />}
      fullWidth
      type="submit"
      size="large"
      {...props}
    />
  )
}

const OIDCButton = ({
  authMethods,
  ...buttonProps
}: ComponentProps<typeof Button> & { authMethods: AuthMethods }) => {
  return (
    <Button
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
      {...buttonProps}
    />
  )
}

const getOIDCLabel = (authMethods: AuthMethods) => {
  return authMethods.oidc.signInText || "OpenID Connect"
}

const ConfirmLoginTypeChangeModal = ({
  open,
  loading,
  error,
  onClose,
  onConfirm,
}: {
  open: boolean
  loading: boolean
  error: unknown
  onClose: () => void
  onConfirm: (password: string) => void
}) => {
  const [password, setPassword] = useState("")

  const handleConfirm = () => {
    onConfirm(password)
  }

  return (
    <ConfirmDialog
      open={open}
      onClose={() => {
        onClose()
      }}
      onConfirm={handleConfirm}
      hideCancel={false}
      cancelText="Cancel"
      confirmText="Update"
      title="Change login type"
      confirmLoading={loading}
      description={
        <Stack>
          <Typography>
            After changing your login type, you will not be able to change it
            again. Are you sure you want to proceed and change your login type?
          </Typography>
          <TextField
            autoFocus
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                handleConfirm()
              }
            }}
            error={Boolean(error)}
            helperText={
              error
                ? getErrorMessage(error, "Your password is incorrect")
                : undefined
            }
            name="confirm-password"
            id="confirm-password"
            value={password}
            onChange={(e) => setPassword(e.currentTarget.value)}
            label="Confirm your password"
            type="password"
          />
        </Stack>
      }
    />
  )
}

export default AccountPage
