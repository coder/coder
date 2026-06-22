import { PlusIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Link as RouterLink } from "react-router";
import type { APIKeyWithOwner } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { ConfirmDeleteDialog } from "./ConfirmDeleteDialog";
import { useTokensData } from "./hooks";
import { TokensPageView } from "./TokensPageView";

const cliCreateCommand = "coder tokens create";

const TokensPage: FC = () => {
	const [tokenToDelete, setTokenToDelete] = useState<
		APIKeyWithOwner | undefined
	>(undefined);

	const {
		data: tokens,
		error: getTokensError,
		isFetching,
		isFetched,
		queryKey,
	} = useTokensData({
		// we currently do not show all tokens in the UI, even if
		// the user has read all permissions
		include_all: false,
		include_expired: false,
	});

	return (
		<>
			<SettingsHeader
				actions={
					<Button asChild variant="outline">
						<RouterLink to="new">
							<PlusIcon />
							Add token
						</RouterLink>
					</Button>
				}
			>
				<SettingsHeaderTitle>Tokens</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Tokens are used to authenticate with the Coder API. You can create a
					token with the Coder CLI using the{" "}
					<code className="bg-surface-secondary text-content-primary text-xs px-1 py-0.5 rounded-sm">
						{cliCreateCommand}
					</code>{" "}
					command.
				</SettingsHeaderDescription>
			</SettingsHeader>
			<TokensPageView
				tokens={tokens}
				isLoading={isFetching}
				hasLoaded={isFetched}
				getTokensError={getTokensError}
				onDelete={(token) => {
					setTokenToDelete(token);
				}}
			/>
			<ConfirmDeleteDialog
				queryKey={queryKey}
				token={tokenToDelete}
				setToken={setTokenToDelete}
			/>
		</>
	);
};

export default TokensPage;
