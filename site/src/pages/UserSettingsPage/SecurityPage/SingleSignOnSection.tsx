import { useState } from "react";
import { Section } from "components/SettingsLayout/Section";
import TextField from "@mui/material/TextField";
import Box from "@mui/material/Box";
import GitHubIcon from "@mui/icons-material/GitHub";
import KeyIcon from "@mui/icons-material/VpnKey";
import Button from "@mui/material/Button";
import Typography from "@mui/material/Typography";
import { convertToOAUTH } from "api/api";
import type {
  AuthMethods,
  LoginType,
  OIDCAuthMethod,
  UserLoginType,
} from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
import { useMutation } from "react-query";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { getErrorMessage } from "api/errors";
import CheckCircleOutlined from "@mui/icons-material/CheckCircleOutlined";
import { EmptyState } from "components/EmptyState/EmptyState";
import Link from "@mui/material/Link";
import { docs } from "utils/docs";

type LoginTypeConfirmation =
  | {
      open: false;
      selectedType: undefined;
    }
  | {
      open: true;
      selectedType: LoginType;
    };

export const redirectToOIDCAuth = (
  toType: string,
  stateString: string,
  redirectTo: string,
) => {
  switch (toType) {
    case "github":
      window.location.href = `/api/v2/users/oauth2/github/callback?oidc_merge_state=${stateString}&redirect=${redirectTo}`;
      break;
    case "oidc":
      window.location.href = `/api/v2/users/oidc/callback?oidc_merge_state=${stateString}&redirect=${redirectTo}`;
      break;
    default:
      throw new Error(`Unknown login type ${toType}`);
  }
};

export const useSingleSignOnSection = () => {
  const [loginTypeConfirmation, setLoginTypeConfirmation] =
    useState<LoginTypeConfirmation>({ open: false, selectedType: undefined });

  const mutation = useMutation(convertToOAUTH, {
    onSuccess: (data) => {
      const loginTypeMsg =
        data.to_type === "github" ? "Github" : "OpenID Connect";
      redirectToOIDCAuth(
        data.to_type,
        data.state_string,
        // The redirect on success should be back to the login page with a nice message.
        // The user should be logged out if this worked.
        encodeURIComponent(
          `/login?info=Login type has been changed to ${loginTypeMsg}. Log in again using the new method.`,
        ),
      );
    },
  });

  const openConfirmation = (selectedType: LoginType) => {
    setLoginTypeConfirmation({ open: true, selectedType });
  };

  const closeConfirmation = () => {
    setLoginTypeConfirmation({ open: false, selectedType: undefined });
    mutation.reset();
  };

  const confirm = (password: string) => {
    if (!loginTypeConfirmation.selectedType) {
      throw new Error("No login type selected");
    }
    mutation.mutate({
      to_type: loginTypeConfirmation.selectedType,
      password,
    });
  };

  return {
    openConfirmation,
    closeConfirmation,
    confirm,
    // We still want to show it loading when it is success so the modal does not
    // change until the redirect
    isUpdating: mutation.isLoading || mutation.isSuccess,
    isConfirming: loginTypeConfirmation.open,
    error: mutation.error,
  };
};

function SSOEmptyState() {
  return (
    <EmptyState
      css={(theme) => ({
        minHeight: 0,
        padding: "48px 32px",
        backgroundColor: theme.palette.background.paper,
        borderRadius: 8,
      })}
      message="No SSO Providers"
      description="No SSO providers are configured with this Coder deployment."
      cta={
        <Link href={docs("/admin/auth")} target="_blank" rel="noreferrer">
          Learn how to add a provider
        </Link>
      }
    />
  );
}

type SingleSignOnSectionProps = ReturnType<typeof useSingleSignOnSection> & {
  authMethods: AuthMethods;
  userLoginType: UserLoginType;
};

export const SingleSignOnSection = ({
  authMethods,
  userLoginType,
  openConfirmation,
  closeConfirmation,
  confirm,
  isUpdating,
  isConfirming,
  error,
}: SingleSignOnSectionProps) => {
  const authList = Object.values(
    authMethods,
  ) as (typeof authMethods)[keyof typeof authMethods][];

  const noSsoEnabled = !authList.some((method) => method.enabled);

  return (
    <>
      <Section
        id="sso-section"
        title="Single Sign On"
        description="Authenticate in Coder using one-click"
      >
        <Box display="grid" gap="16px">
          {userLoginType.login_type === "password" ? (
            <>
              {authMethods.github.enabled && (
                <Button
                  size="large"
                  fullWidth
                  disabled={isUpdating}
                  startIcon={<GitHubIcon sx={{ width: 16, height: 16 }} />}
                  onClick={() => openConfirmation("github")}
                >
                  GitHub
                </Button>
              )}

              {authMethods.oidc.enabled && (
                <Button
                  size="large"
                  fullWidth
                  disabled={isUpdating}
                  startIcon={<OIDCIcon oidcAuth={authMethods.oidc} />}
                  onClick={() => openConfirmation("oidc")}
                >
                  {getOIDCLabel(authMethods.oidc)}
                </Button>
              )}

              {noSsoEnabled && <SSOEmptyState />}
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
                  {userLoginType.login_type === "github"
                    ? "GitHub"
                    : getOIDCLabel(authMethods.oidc)}
                </strong>
              </span>
              <Box sx={{ ml: "auto", lineHeight: 1 }}>
                {userLoginType.login_type === "github" ? (
                  <GitHubIcon sx={{ width: 16, height: 16 }} />
                ) : (
                  <OIDCIcon oidcAuth={authMethods.oidc} />
                )}
              </Box>
            </Box>
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
  );
};

const OIDCIcon = ({ oidcAuth }: { oidcAuth: OIDCAuthMethod }) => {
  if (!oidcAuth.iconUrl) {
    return <KeyIcon sx={{ width: 16, height: 16 }} />;
  }

  return (
    <Box
      component="img"
      alt="Open ID Connect icon"
      src={oidcAuth.iconUrl}
      sx={{ width: 16, height: 16 }}
    />
  );
};

const getOIDCLabel = (oidcAuth: OIDCAuthMethod) => {
  return oidcAuth.signInText || "OpenID Connect";
};

const ConfirmLoginTypeChangeModal = ({
  open,
  loading,
  error,
  onClose,
  onConfirm,
}: {
  open: boolean;
  loading: boolean;
  error: unknown;
  onClose: () => void;
  onConfirm: (password: string) => void;
}) => {
  const [password, setPassword] = useState("");

  const handleConfirm = () => {
    onConfirm(password);
  };

  return (
    <ConfirmDialog
      open={open}
      onClose={() => {
        onClose();
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
                handleConfirm();
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
  );
};
