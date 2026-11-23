import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { TrashIcon } from "lucide-react";
import type { APIKeyWithOwner } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";

dayjs.extend(relativeTime);

const lastUsedOrNever = (lastUsed: string) => {
	const t = dayjs(lastUsed);
	return t.valueOf() > 0 ? t.fromNow() : "Never";
};

type TokensPageViewProps = {
	tokens?: APIKeyWithOwner[];
	getTokensError?: unknown;
	isFetching: boolean;
	hasLoaded: boolean;
	onDelete: (token: APIKeyWithOwner) => void;
	deleteTokenError?: unknown;
	children?: React.ReactNode;
};

export const TokensPageView: React.FC<TokensPageViewProps> = ({
	tokens,
	getTokensError,
	isFetching,
	hasLoaded,
	onDelete,
	deleteTokenError,
}) => {
	return (
		<div className="flex flex-col gap-4">
			{Boolean(getTokensError) && <ErrorAlert error={getTokensError} />}
			{Boolean(deleteTokenError) && <ErrorAlert error={deleteTokenError} />}

			<Table>
				<TableHeader>
					<TableRow>
						<TableHead className="w-1/5">ID</TableHead>
						<TableHead className="w-1/5">Name</TableHead>
						<TableHead className="w-1/5">Last used</TableHead>
						<TableHead className="w-1/5">Expires at</TableHead>
						<TableHead className="w-1/5">Created at</TableHead>
						<TableHead className="w-[1%]" />
					</TableRow>
				</TableHeader>
				<TableBody>
					<TokensTableBody
						tokens={tokens}
						isFetching={isFetching}
						hasLoaded={hasLoaded}
						onDelete={onDelete}
					/>
				</TableBody>
			</Table>
		</div>
	);
};

type TokensTableBodyProps = {
	tokens?: APIKeyWithOwner[];
	isFetching: boolean;
	hasLoaded: boolean;
	onDelete: (token: APIKeyWithOwner) => void;
};

const TokensTableBody: React.FC<TokensTableBodyProps> = ({
	tokens,
	isFetching,
	hasLoaded,
	onDelete,
}) => {
	if (isFetching && !tokens) {
		return <TableLoader />;
	}
	if (hasLoaded && (!tokens || tokens.length === 0)) {
		return <TableEmpty message="No tokens found" />;
	}
	return tokens?.map((token) => (
		<TableRow key={token.id} data-testid={`token-${token.id}`} tabIndex={0}>
			<TableCell>
				<span className="text-content-secondary">{token.id}</span>
			</TableCell>

			<TableCell>
				<span className="text-content-secondary">{token.token_name}</span>
			</TableCell>

			<TableCell>{lastUsedOrNever(token.last_used)}</TableCell>

			<TableCell>
				<span className="text-content-secondary" data-pixel="ignore">
					{dayjs(token.expires_at).fromNow()}
				</span>
			</TableCell>

			<TableCell>
				<span className="text-content-secondary">
					{dayjs(token.created_at).fromNow()}
				</span>
			</TableCell>

			<TableCell>
				<span className="text-content-secondary">
					<Button
						onClick={() => {
							onDelete(token);
						}}
						size="icon"
						variant="destructive"
						aria-label="Delete token"
					>
						<TrashIcon className="size-icon-sm" />
					</Button>
				</span>
			</TableCell>
		</TableRow>
	));
};
