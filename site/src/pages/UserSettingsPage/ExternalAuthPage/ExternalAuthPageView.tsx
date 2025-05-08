import { useTheme } from "@emotion/react";
import LoadingButton from "@mui/lab/LoadingButton";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import Tooltip from "@mui/material/Tooltip";
import visuallyHidden from "@mui/utils/visuallyHidden";
import { externalAuthProvider } from "api/queries/externalAuth";
import type {
	ExternalAuthLink,
	ExternalAuthLinkProvider,
	ListUserExternalAuthResponse,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Avatar } from "components/Avatar/Avatar";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { AutorenewIcon } from "lucide-react";
import { EllipsisVertical } from "lucide-react";
import type { ExternalAuthPollingState } from "pages/CreateWorkspacePage/CreateWorkspacePage";
import { type FC, useCallback, useEffect, useState } from "react";
import { useQuery } from "react-query";

export type ExternalAuthPageViewProps = {
	isLoading: boolean;
	getAuthsError?: unknown;
	unlinked: number;
	auths?: ListUserExternalAuthResponse;
	onUnlinkExternalAuth: (provider: string) => void;
	onValidateExternalAuth: (provider: string) => void;
};

export const ExternalAuthPageView: FC<ExternalAuthPageViewProps> = ({
	isLoading,
	getAuthsError,
	auths,
	unlinked,
	onUnlinkExternalAuth,
	onValidateExternalAuth,
}) => {
	if (getAuthsError) {
		// Nothing to show if there is an error
		return <ErrorAlert error={getAuthsError} />;
	}

	if (isLoading || !auths) {
		return <Loader fullscreen />;
	}

	return (
		<>
			<TableContainer>
				<Table>
					<TableHead>
						<TableRow>
							<TableCell>Application</TableCell>
							<TableCell>
								<span aria-hidden css={{ ...visuallyHidden }}>
									Link to connect
								</span>
							</TableCell>
							<TableCell width="1%" />
						</TableRow>
					</TableHead>
					<TableBody>
						{auths.providers === null || auths.providers?.length === 0 ? (
							<TableEmpty message="No providers have been configured" />
						) : (
							auths.providers?.map((app) => (
								<ExternalAuthRow
									key={app.id}
									app={app}
									unlinked={unlinked}
									link={auths.links.find((l) => l.provider_id === app.id)}
									onUnlinkExternalAuth={() => {
										onUnlinkExternalAuth(app.id);
									}}
									onValidateExternalAuth={() => {
										onValidateExternalAuth(app.id);
									}}
								/>
							))
						)}
					</TableBody>
				</Table>
			</TableContainer>
		</>
	);
};

interface ExternalAuthRowProps {
	app: ExternalAuthLinkProvider;
	link?: ExternalAuthLink;
	unlinked: number;
	onUnlinkExternalAuth: () => void;
	onValidateExternalAuth: () => void;
}

const ExternalAuthRow: FC<ExternalAuthRowProps> = ({
	app,
	unlinked,
	link,
	onUnlinkExternalAuth,
	onValidateExternalAuth,
}) => {
	const theme = useTheme();
	const name = app.display_name || app.id || app.type;
	const authURL = `/external-auth/${app.id}`;

	const {
		externalAuth,
		externalAuthPollingState,
		refetch,
		startPollingExternalAuth,
	} = useExternalAuth(app.id, unlinked);

	const authenticated = externalAuth
		? externalAuth.authenticated
		: (link?.authenticated ?? false);

	return (
		<TableRow key={app.id}>
			<TableCell>
				<Stack direction="row" alignItems="center" spacing={1}>
					<Avatar variant="icon" src={app.display_icon} fallback={name} />
					<span className="font-semibold">{name}</span>
					{/*
					 * If the link is authenticated and has a refresh token, show that it will automatically
					 * attempt to authenticate when the token expires.
					 */}
					{link?.has_refresh_token && authenticated && (
						<Tooltip
							title="Authentication token will automatically refresh when expired."
							placement="right"
						>
							<AutorenewIcon
								sx={{
									fontSize: "0.75rem",
								}}
							/>
						</Tooltip>
					)}

					{link?.validate_error && (
						<span>
							<span
								css={{ paddingLeft: "1em", color: theme.palette.error.light }}
							>
								Error:{" "}
							</span>
							{link?.validate_error}
						</span>
					)}
				</Stack>
			</TableCell>
			<TableCell css={{ textAlign: "right" }}>
				<LoadingButton
					disabled={authenticated}
					variant="contained"
					loading={externalAuthPollingState === "polling"}
					onClick={() => {
						window.open(authURL, "_blank", "width=900,height=600");
						startPollingExternalAuth();
					}}
				>
					{authenticated ? "Authenticated" : "Click to Login"}
				</LoadingButton>
			</TableCell>
			<TableCell>
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button size="icon-lg" variant="subtle" aria-label="Open menu">
							<EllipsisVertical aria-hidden="true" />
							<span className="sr-only">Open menu</span>
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end">
						<DropdownMenuItem
							onClick={async () => {
								onValidateExternalAuth();
								// This is kinda jank. It does a refetch of the thing
								// it just validated... But we need to refetch to update the
								// login button. And the 'onValidateExternalAuth' does the
								// message display.
								await refetch();
							}}
						>
							Test Validate&hellip;
						</DropdownMenuItem>
						<DropdownMenuItem
							className="text-content-destructive focus:text-content-destructive"
							onClick={async () => {
								onUnlinkExternalAuth();
								await refetch();
							}}
						>
							Unlink&hellip;
						</DropdownMenuItem>
					</DropdownMenuContent>
				</DropdownMenu>
			</TableCell>
		</TableRow>
	);
};

// useExternalAuth handles the polling of the auth to update the button.
const useExternalAuth = (providerID: string, unlinked: number) => {
	const [externalAuthPollingState, setExternalAuthPollingState] =
		useState<ExternalAuthPollingState>("idle");

	const startPollingExternalAuth = useCallback(() => {
		setExternalAuthPollingState("polling");
	}, []);

	const { data: externalAuth, refetch } = useQuery({
		...externalAuthProvider(providerID),
		refetchInterval: externalAuthPollingState === "polling" ? 1000 : false,
	});

	const signedIn = externalAuth?.authenticated;

	useEffect(() => {
		if (unlinked > 0) {
			void refetch();
		}
	}, [refetch, unlinked]);

	useEffect(() => {
		if (signedIn) {
			setExternalAuthPollingState("idle");
			return;
		}

		if (externalAuthPollingState !== "polling") {
			return;
		}

		// Poll for a maximum of one minute
		const quitPolling = setTimeout(
			() => setExternalAuthPollingState("abandoned"),
			60_000,
		);
		return () => {
			clearTimeout(quitPolling);
		};
	}, [externalAuthPollingState, signedIn]);

	return {
		startPollingExternalAuth,
		externalAuth,
		externalAuthPollingState,
		refetch,
	};
};
