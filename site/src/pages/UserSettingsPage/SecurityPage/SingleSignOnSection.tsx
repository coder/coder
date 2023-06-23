import { useState } from "react"
import { Section } from "../../../components/SettingsLayout/Section"
import { useMe } from "hooks/useMe"
import TextField from "@mui/material/TextField"
import Box from "@mui/material/Box"
import GitHubIcon from "@mui/icons-material/GitHub"
import KeyIcon from "@mui/icons-material/VpnKey"
import Button from "@mui/material/Button"
import { useLocation } from "react-router-dom"
import { retrieveRedirect } from "utils/redirect"
import Typography from "@mui/material/Typography"
import { convertToOAUTH } from "api/api"
import { AuthMethods, LoginType } from "api/typesGenerated"
import Skeleton from "@mui/material/Skeleton"
import { Stack } from "components/Stack/Stack"
import { useMutation } from "@tanstack/react-query"
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import { getErrorMessage } from "api/errors"
import CheckCircleOutlined from "@mui/icons-material/CheckCircleOutlined"

type LoginTypeConfirmation =
  | {
      open: false
      selectedType: undefined
    }
  | {
      open: true
      selectedType: LoginType
    }

export const useSingleSignOnSection = () => {
  const me = useMe()
  const location = useLocation()
  const redirectTo = retrieveRedirect(location.search)
  const [loginTypeConfirmation, setLoginTypeConfirmation] =
    useState<LoginTypeConfirmation>({ open: false, selectedType: undefined })

  const mutation = useMutation(convertToOAUTH, {
    onSuccess: (data) => {
      window.location.href = `/api/v2/users/oidc/callback?oidc_merge_state=${
        data.state_string
      }&redirect=${encodeURIComponent(redirectTo)}`
    },
  })

  const openConfirmation = (selectedType: LoginType) => {
    setLoginTypeConfirmation({ open: true, selectedType })
  }

  const closeConfirmation = () => {
    setLoginTypeConfirmation({ open: false, selectedType: undefined })
    mutation.reset()
  }

  const confirm = (password: string) => {
    if (!loginTypeConfirmation.selectedType) {
      throw new Error("No login type selected")
    }
    mutation.mutate({
      to_login_type: loginTypeConfirmation.selectedType,
      email: me.email,
      password,
    })
  }

  return {
    openConfirmation,
    closeConfirmation,
    confirm,
    // We still want to show it loading when it is success so the modal does not
    // change until the redirect
    isUpdating: mutation.isLoading || mutation.isSuccess,
    isConfirming: loginTypeConfirmation.open,
    error: mutation.error,
  }
}

type SingleSignOnSectionProps = ReturnType<typeof useSingleSignOnSection> & {
  authMethods: AuthMethods
}

export const SingleSignOnSection = ({
  authMethods,
  openConfirmation,
  closeConfirmation,
  confirm,
  isUpdating,
  isConfirming,
  error,
}: SingleSignOnSectionProps) => {
  return (
    <>
      <Section
        title="Single Sign On"
        description="Authenticate in Coder using one-click"
      >
        <Box display="grid" gap="16px">
          {authMethods ? (
            authMethods.me_login_type === "password" ? (
              <>
                {authMethods.github.enabled && (
                  <Button
                    disabled={isUpdating}
                    onClick={() => openConfirmation("github")}
                    startIcon={<GitHubIcon sx={{ width: 16, height: 16 }} />}
                    fullWidth
                    size="large"
                  >
                    GitHub
                  </Button>
                )}
                {authMethods.oidc.enabled && (
                  <Button
                    size="large"
                    startIcon={<OIDCIcon authMethods={authMethods} />}
                    fullWidth
                    disabled={isUpdating}
                    onClick={() => openConfirmation("oidc")}
                  >
                    {getOIDCLabel(authMethods)}
                  </Button>
                )}
              </>
            ) : (
              <Box
                sx={{
                  background: (theme) => theme.palette.background.paper,
                  borderRadius: 1,
                  border: (theme) => `1px solid ${theme.palette.divider}`,
                  padding: 2,
                  display: "flex",
                  gap: 2,
                  alignItems: "center",
                  fontSize: 14,
                }}
              >
                <CheckCircleOutlined
                  sx={{
                    color: (theme) => theme.palette.success.light,
                    fontSize: 16,
                  }}
                />
                <span>
                  Authenticated with{" "}
                  <strong>
                    {authMethods.me_login_type === "github"
                      ? "GitHub"
                      : getOIDCLabel(authMethods)}
                  </strong>
                </span>
                <Box sx={{ ml: "auto", lineHeight: 1 }}>
                  {authMethods.me_login_type === "github" ? (
                    <GitHubIcon sx={{ width: 16, height: 16 }} />
                  ) : (
                    <OIDCIcon authMethods={authMethods} />
                  )}
                </Box>
              </Box>
            )
          ) : (
            <Skeleton
              variant="rectangular"
              sx={{ height: 40, borderRadius: 1 }}
            />
          )}
        </Box>
      </Section>

      <ConfirmLoginTypeChangeModal
        open={isConfirming}
        error={error}
        loading={isUpdating}
        onClose={closeConfirmation}
        onConfirm={confirm}
      />
    </>
  )
}

const OIDCIcon = ({ authMethods }: { authMethods: AuthMethods }) => {
  return authMethods.oidc.iconUrl ? (
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
