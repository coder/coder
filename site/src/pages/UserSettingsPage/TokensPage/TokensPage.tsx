import type { APIKeyWithOwner } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Stack } from "components/Stack/Stack";
import { PlusIcon } from "lucide-react";
import { type FC, useState } from "react";
import { Link as RouterLink } from "react-router";
import { cn } from "utils/cn";
import { Section } from "../Section";
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
	});

	return (
		<>
			<Section
				title="Tokens"
				className={cn([
					"[&_code]:bg-surface-primary [&_code]:text-content-primary",
					"[&_code]:text-xs [&_code]:py-0.5 [&_code]:px-1 [&_code]:rounded-sm",
				])}
				description={
					<>
						Tokens are used to authenticate with the Coder API. You can create a
						token with the Coder CLI using the <code>{cliCreateCommand}</code>{" "}
						command.
					</>
				}
				layout="fluid"
			>
				<TokenActions />
				<TokensPageView
					tokens={tokens}
					isLoading={isFetching}
					hasLoaded={isFetched}
					getTokensError={getTokensError}
					onDelete={(token) => {
						setTokenToDelete(token);
					}}
				/>
			</Section>
			<ConfirmDeleteDialog
				queryKey={queryKey}
				token={tokenToDelete}
				setToken={setTokenToDelete}
			/>
		</>
	);
};

const TokenActions: FC = () => (
	<Stack direction="row" justifyContent="end" className="mb-2">
		<Button asChild variant="outline">
			<RouterLink to="new">
				<PlusIcon />
				Add token
			</RouterLink>
		</Button>
	</Stack>
);

export default TokensPage;
