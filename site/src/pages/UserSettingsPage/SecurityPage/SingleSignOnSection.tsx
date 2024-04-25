import { useTheme } from "@emotion/react";
import CheckCircleOutlined from "@mui/icons-material/CheckCircleOutlined";
import GitHubIcon from "@mui/icons-material/GitHub";
import KeyIcon from "@mui/icons-material/VpnKey";
import Button from "@mui/material/Button";
import Link from "@mui/material/Link";
import TextField from "@mui/material/TextField";
import { type FC, useState } from "react";
import { useMutation } from "react-query";
import { convertToOAUTH } from "api/api";
import { getErrorMessage } from "api/errors";
import type {
  AuthMethods,
  LoginType,
  OIDCAuthMethod,
  UserLoginType,
} from "api/typesGenerated";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Stack } from "components/Stack/Stack";
import { docs } from "utils/docs";
import { Section } from "../Section";

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
          `/login?message=Login type has been changed to ${loginTypeMsg}. Log in again using the new method.`,
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

const SSOEmptyState: FC = () => {
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
};

type SingleSignOnSectionProps = ReturnType<typeof useSingleSignOnSection> & {
  authMethods: AuthMethods;
  userLoginType: UserLoginType;
};

export const SingleSignOnSection: FC<SingleSignOnSectionProps> = ({
  authMethods,
  userLoginType,
  openConfirmation,
  closeConfirmation,
  confirm,
  isUpdating,
  isConfirming,
  error,
}) => {
  const theme = useTheme();

  const noSsoEnabled = !authMethods.github.enabled && !authMethods.oidc.enabled;

  return (
    <>
      <Section
        id="sso-section"
        title="Single Sign On"
        description="Authenticate in Coder using one-click"
      >
        <div css={{ display: "grid", gap: "16px" }}>
          {userLoginType.login_type === "password" ? (
            <>
              {authMethods.github.enabled && (
                <Button
                  size="large"
                  fullWidth
                  disabled={isUpdating}
                  startIcon={<GitHubIcon css={{ width: 16, height: 16 }} />}
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
            <div
              css={{
                background: theme.palette.background.paper,
                borderRadius: 8,
                border: `1px solid ${theme.palette.divider}`,
                padding: 16,
                display: "flex",
                gap: 16,
                alignItems: "center",
                fontSize: 14,
              }}
            >
              <CheckCircleOutlined
                css={{
                  color: theme.palette.success.light,
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
              <div css={{ marginLeft: "auto", lineHeight: 1 }}>
                {userLoginType.login_type === "github" ? (
                  <GitHubIcon css={{ width: 16, height: 16 }} />
                ) : (
                  <OIDCIcon oidcAuth={authMethods.oidc} />
                )}
              </div>
            </div>
          )}
        </div>
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

interface OIDCIconProps {
  oidcAuth: OIDCAuthMethod;
}

const OIDCIcon: FC<OIDCIconProps> = ({ oidcAuth }) => {
  if (!oidcAuth.iconUrl) {
    return <KeyIcon css={{ width: 16, height: 16 }} />;
  }

  return (
    <img
      alt="Open ID Connect icon"
      src={oidcAuth.iconUrl}
      css={{ width: 16, height: 16 }}
    />
  );
};

const getOIDCLabel = (oidcAuth: OIDCAuthMethod) => {
  return oidcAuth.signInText || "OpenID Connect";
};

interface ConfirmLoginTypeChangeModalProps {
  open: boolean;
  loading: boolean;
  error: unknown;
  onClose: () => void;
  onConfirm: (password: string) => void;
}

const ConfirmLoginTypeChangeModal: FC<ConfirmLoginTypeChangeModalProps> = ({
  open,
  loading,
  error,
  onClose,
  onConfirm,
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
        <Stack spacing={4}>
          <p>
            After changing your login type, you will not be able to change it
            again. Are you sure you want to proceed and change your login type?
          </p>
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
