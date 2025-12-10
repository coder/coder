import Link from "@mui/material/Link";
import TextField from "@mui/material/TextField";
import { API } from "api/api";
import { getErrorMessage } from "api/errors";
import type {
	AuthMethods,
	LoginType,
	OIDCAuthMethod,
	UserLoginType,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { EmptyState } from "components/EmptyState/EmptyState";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import { Stack } from "components/Stack/Stack";
import { CircleCheck as CircleCheckIcon, KeyIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useMutation } from "react-query";
import { cn } from "utils/cn";
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

	const mutation = useMutation({
		mutationFn: API.convertToOAUTH,
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
		isUpdating: mutation.isPending || mutation.isSuccess,
		isConfirming: loginTypeConfirmation.open,
		error: mutation.error,
	};
};

const SSOEmptyState: FC = () => {
	return (
		<EmptyState
			className="rounded-lg border border-solid border-border min-h-0"
			message="No SSO Providers"
			description="No SSO providers are configured with this Coder deployment."
			cta={
				<Link
					href={docs("/admin/users/oidc-auth")}
					target="_blank"
					rel="noreferrer"
				>
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
	const noSsoEnabled = !authMethods.github.enabled && !authMethods.oidc.enabled;

	return (
		<>
			<Section
				id="sso-section"
				title="Single Sign On"
				description="Authenticate in Coder using one-click"
			>
				<div className="grid gap-4">
					{userLoginType.login_type === "password" ? (
						<>
							{authMethods.github.enabled && (
								<Button
									variant="outline"
									size="lg"
									className="w-full"
									disabled={isUpdating}
									onClick={() => openConfirmation("github")}
								>
									<ExternalImage src="/icon/github.svg" />
									GitHub
								</Button>
							)}

							{authMethods.oidc.enabled && (
								<Button
									variant="outline"
									size="lg"
									className="w-full"
									disabled={isUpdating}
									onClick={() => openConfirmation("oidc")}
								>
									<OIDCIcon oidcAuth={authMethods.oidc} />
									{getOIDCLabel(authMethods.oidc)}
								</Button>
							)}

							{noSsoEnabled && <SSOEmptyState />}
						</>
					) : (
						<div
							className={cn([
								"text-sm flex items-center gap-4 p-4 rounded-lg border border-solid",
								"bg-surface-secondary border-border dark:border-content-disabled",
							])}
						>
							<CircleCheckIcon className="size-icon-xs text-content-success" />
							<span>
								Authenticated with{" "}
								<strong>
									{userLoginType.login_type === "github"
										? "GitHub"
										: getOIDCLabel(authMethods.oidc)}
								</strong>
							</span>
							<div className="ml-auto leading-none">
								{userLoginType.login_type === "github" ? (
									<ExternalImage src="/icon/github.svg" />
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
		return <KeyIcon />;
	}

	return (
		<img alt="Open ID Connect icon" src={oidcAuth.iconUrl} className="size-4" />
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
