import { css, type Interpolation, type Theme } from "@emotion/react";
import type { APIKeyWithOwner } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Checkbox } from "components/Checkbox/Checkbox";
import { Stack } from "components/Stack/Stack";
import { KeyIcon, LockKeyholeIcon, PlusIcon } from "lucide-react";
import { type FC, useId, useState } from "react";
import { Link as RouterLink } from "react-router";
import { Section } from "../Section";
import { ConfirmDeleteDialog } from "./ConfirmDeleteDialog";
import { useTokensData } from "./hooks";
import { TokensPageView } from "./TokensPageView";

const cliCreateCommand = "coder tokens create";

const TokensPage: FC = () => {
	const [tokenToDelete, setTokenToDelete] = useState<
		APIKeyWithOwner | undefined
	>(undefined);
	const [showExpired, setShowExpired] = useState(false);
	const status = showExpired ? "all" : "active";

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
		status,
	});

	return (
		<>
			<Section
				title="Tokens"
				css={styles.section}
				description={
					<>
						Tokens are used to authenticate with the Coder API. You can create a
						token with the Coder CLI using the <code>{cliCreateCommand}</code>{" "}
						command.
					</>
				}
				layout="fluid"
			>
				<TokenTypeLegend />
				<TokenActions
					showExpired={showExpired}
					onShowExpiredChange={setShowExpired}
				/>

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

const TokenActions: FC<{
	showExpired: boolean;
	onShowExpiredChange: (value: boolean) => void;
}> = ({ showExpired, onShowExpiredChange }) => {
	const checkboxId = useId();

	return (
		<Stack
			direction="row"
			justifyContent="space-between"
			alignItems="center"
			css={{ marginBottom: 8 }}
		>
			<div className="flex items-center gap-2 text-sm text-content-secondary">
				<Checkbox
					id={checkboxId}
					checked={showExpired}
					onCheckedChange={(value) => onShowExpiredChange(value === true)}
					aria-labelledby={`${checkboxId}-label`}
				/>
				<label
					id={`${checkboxId}-label`}
					htmlFor={checkboxId}
					className="cursor-pointer"
				>
					Show expired
				</label>
			</div>
			<Button asChild variant="outline">
				<RouterLink to="new">
					<PlusIcon />
					Add token
				</RouterLink>
			</Button>
		</Stack>
	);
};

const TokenTypeLegend: FC = () => (
	<div className="mb-4 flex flex-wrap items-center gap-4 rounded-md bg-surface-secondary px-4 py-2 text-sm text-content-secondary">
		<span className="flex items-center gap-2">
			<LockKeyholeIcon className="size-4" aria-hidden="true" />
			<span>Password keys: Session tokens from browser login (<code>/cli-auth</code>).
			Created automatically when you authenticate.
			</span>
		</span>
		<span className="flex items-center gap-2">
			<KeyIcon className="size-4" aria-hidden="true" />
			<span>
			Token keys: API tokens for automation and CI/CD. Created manually from CLI, API or this page.
			</span>
		</span>
	</div>
);

const styles = {
	section: (theme) => css`
    & code {
      background: ${theme.palette.divider};
      font-size: 12px;
      padding: 2px 4px;
      color: ${theme.palette.text.primary};
      border-radius: 2px;
    }
  `,
} satisfies Record<string, Interpolation<Theme>>;

export default TokensPage;
