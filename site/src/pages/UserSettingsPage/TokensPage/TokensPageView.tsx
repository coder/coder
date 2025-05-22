import { useTheme } from "@emotion/react";
import IconButton from "@mui/material/IconButton";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type { APIKeyWithOwner } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { Stack } from "components/Stack/Stack";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { TableLoader } from "components/TableLoader/TableLoader";
import { addYears, formatDistanceToNow, isBefore, parseISO } from "date-fns";
import { TrashIcon } from "lucide-react";
import type { FC, ReactNode } from "react";

const lastUsedOrNever = (lastUsed: string) => {
	const t = parseISO(lastUsed);
	const now = new Date();
	return isBefore(now, addYears(t, 100))
		? formatDistanceToNow(t, { addSuffix: true })
		: "Never";
};

export interface TokensPageViewProps {
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
		<Stack>
			{Boolean(getTokensError) && <ErrorAlert error={getTokensError} />}
			{Boolean(deleteTokenError) && <ErrorAlert error={deleteTokenError} />}
			<TableContainer>
				<Table>
					<TableHead>
						<TableRow>
							<TableCell width="20%">ID</TableCell>
							<TableCell width="20%">Name</TableCell>
							<TableCell width="20%">Last Used</TableCell>
							<TableCell width="20%">Expires At</TableCell>
							<TableCell width="20%">Created At</TableCell>
							<TableCell width="0%" />
						</TableRow>
					</TableHead>
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
													{formatDistanceToNow(parseISO(token.expires_at), {
														addSuffix: true,
													})}
												</span>
											</TableCell>

											<TableCell>
												<span style={{ color: theme.palette.text.secondary }}>
													{formatDistanceToNow(parseISO(token.created_at), {
														addSuffix: true,
													})}
												</span>
											</TableCell>

											<TableCell>
												<span style={{ color: theme.palette.text.secondary }}>
													<IconButton
														onClick={() => {
															onDelete(token);
														}}
														size="medium"
														aria-label="Delete token"
													>
														<TrashIcon className="size-icon-sm" />
													</IconButton>
												</span>
											</TableCell>
										</TableRow>
									);
								})}
							</Cond>
						</ChooseOne>
					</TableBody>
				</Table>
			</TableContainer>
		</Stack>
	);
};
