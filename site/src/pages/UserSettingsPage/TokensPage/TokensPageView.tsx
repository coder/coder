import { useTheme } from "@emotion/react";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { TrashIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import type { APIKeyWithOwner } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ChooseOne, Cond } from "#/components/Conditionals/ChooseOne";
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
	const now = dayjs();
	return now.isBefore(t.add(100, "year")) ? t.fromNow() : "Never";
};

interface TokensPageViewProps {
	tokens?: APIKeyWithOwner[];
	getTokensError?: unknown;
	isLoading: boolean;
	hasLoaded: boolean;
	onDelete: (token: APIKeyWithOwner) => void;
	deleteTokenError?: unknown;
	children?: ReactNode;
}

export const TokensPageView: FC<TokensPageViewProps> = ({
	tokens,
	getTokensError,
	isLoading,
	hasLoaded,
	onDelete,
	deleteTokenError,
}) => {
	const theme = useTheme();

	return (
		<div className="flex flex-col gap-4">
			{Boolean(getTokensError) && <ErrorAlert error={getTokensError} />}
			{Boolean(deleteTokenError) && <ErrorAlert error={deleteTokenError} />}

			<Table>
				<TableHeader>
					<TableRow>
						<TableHead className="w-1/5">ID</TableHead>
						<TableHead className="w-1/5">Name</TableHead>
						<TableHead className="w-1/5">Last Used</TableHead>
						<TableHead className="w-1/5">Expires At</TableHead>
						<TableHead className="w-1/5">Created At</TableHead>
						<TableHead className="w-[1%]" />
					</TableRow>
				</TableHeader>
				<TableBody>
					<ChooseOne>
						<Cond condition={isLoading}>
							<TableLoader />
						</Cond>
						<Cond condition={hasLoaded && (!tokens || tokens.length === 0)}>
							<TableEmpty message="No tokens found" />
						</Cond>
						<Cond>
							{tokens?.map((token) => {
								return (
									<TableRow
										key={token.id}
										data-testid={`token-${token.id}`}
										tabIndex={0}
									>
										<TableCell>
											<span style={{ color: theme.palette.text.secondary }}>
												{token.id}
											</span>
										</TableCell>

										<TableCell>
											<span style={{ color: theme.palette.text.secondary }}>
												{token.token_name}
											</span>
										</TableCell>

										<TableCell>{lastUsedOrNever(token.last_used)}</TableCell>

										<TableCell>
											<span
												style={{ color: theme.palette.text.secondary }}
												data-chromatic="ignore"
											>
												{dayjs(token.expires_at).fromNow()}
											</span>
										</TableCell>

										<TableCell>
											<span style={{ color: theme.palette.text.secondary }}>
												{dayjs(token.created_at).fromNow()}
											</span>
										</TableCell>

										<TableCell>
											<span style={{ color: theme.palette.text.secondary }}>
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
								);
							})}
						</Cond>
					</ChooseOne>
				</TableBody>
			</Table>
		</div>
	);
};
