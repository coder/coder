import { API } from "api/api";
import type { APIAllowListTarget, APIKeyWithOwner } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { Stack } from "components/Stack/Stack";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { TableEmpty } from "components/TableEmpty/TableEmpty";
import { TableLoader } from "components/TableLoader/TableLoader";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { PencilIcon, TrashIcon } from "lucide-react";
import { type FC, type ReactNode, useMemo } from "react";
import { useQueries } from "react-query";
import { Link as RouterLink } from "react-router";

dayjs.extend(relativeTime);

const lastUsedOrNever = (lastUsed?: string | null) => {
	if (!lastUsed) {
		return "Never";
	}

	const parsed = dayjs(lastUsed);
	if (!parsed.isValid()) {
		return "Never";
	}

	const now = dayjs();
	return now.isBefore(parsed.add(100, "year")) ? parsed.fromNow() : "Never";
};

const getScopeList = (token?: APIKeyWithOwner | null) => {
	if (!token) {
		return ["coder:all (implicit)"];
	}

	const scopes =
		token.scopes && token.scopes.length > 0
			? token.scopes
			: token.scope
				? [token.scope]
				: [];

	if (scopes.length === 0) {
		return ["coder:all (implicit)"];
	}

	return scopes;
};

type AllowListFetcher = (id: string) => Promise<string>;

const allowListFetchers: Partial<Record<string, AllowListFetcher>> = {
	template: async (templateId: string) => {
		const template = await API.getTemplate(templateId);
		return template.display_name?.trim() || template.name || templateId;
	},
	workspace: async (workspaceId: string) => {
		const workspace = await API.getWorkspace(workspaceId);
		return workspace.name?.trim() || workspaceId;
	},
};

const normalizeAllowListEntry = (
	entry: APIAllowListTarget | string | null | undefined,
) => {
	if (entry === null || entry === undefined) {
		return "*:*";
	}

	if (typeof entry === "string") {
		return entry;
	}

	const type = entry.type ?? "*";
	const id = entry.id ?? "*";

	return `${type}:${id}`;
};

const parseAllowListKey = (key: string) => {
	const [type = "*", id = "*"] = key.split(":", 2);
	return { type, id };
};

const formatResourceType = (rawType: string) => {
	if (rawType === "*") {
		return "Any resource";
	}
	return rawType
		.split(/[_-]/g)
		.filter(Boolean)
		.map((segment) => segment.toLowerCase())
		.join(" ");
};

const pluralizeResourceType = (type: string) => {
	if (type === "*") {
		return "resources";
	}
	const formatted = formatResourceType(type);
	return /s$/i.test(formatted) ? formatted : `${formatted}s`;
};

const buildAllowListLabel = (
	type: string,
	id: string,
	options?: { resolvedName?: string },
) => {
	if (type === "*" && id === "*") {
		return "Any resource";
	}
	if (id === "*") {
		return `All ${pluralizeResourceType(type)}`;
	}
	if (type === "*") {
		return `Any resource (${id})`;
	}
	const resolved = options?.resolvedName?.trim();
	if (resolved) {
		return `${formatResourceType(type)}: ${resolved}`;
	}
	return `${formatResourceType(type)}: ${id}`;
};

const defaultLabelForKey = (key: string) => {
	const { type, id } = parseAllowListKey(key);
	return buildAllowListLabel(type, id);
};

const useAllowListLabelMap = (tokens?: APIKeyWithOwner[] | null) => {
	const uniqueEntries = useMemo(() => {
		const accumulator = new Map<
			string,
			{ key: string; type: string; id: string; displayName?: string }
		>();
		tokens?.forEach((token) => {
			token.allow_list?.forEach((entry) => {
				const key = normalizeAllowListEntry(entry);
				if (!key) {
					return;
				}

				if (!accumulator.has(key)) {
					const { type, id } = parseAllowListKey(key);
					const displayName =
						typeof entry === "string"
							? undefined
							: entry.display_name?.trim() || undefined;
					accumulator.set(key, { key, type, id, displayName });
					return;
				}

				const existing = accumulator.get(key);
				if (existing && !existing.displayName && typeof entry !== "string") {
					const displayName = entry.display_name?.trim() || undefined;
					if (displayName) {
						accumulator.set(key, { ...existing, displayName });
					}
				}
			});
		});
		return Array.from(accumulator.values());
	}, [tokens]);

	const entriesRequiringLookup = useMemo(
		() =>
			uniqueEntries.filter(
				({ type, id, displayName }) =>
					type !== "*" &&
					id !== "*" &&
					!displayName &&
					Boolean(allowListFetchers[type]),
			),
		[uniqueEntries],
	);

	const lookupResults = useQueries({
		queries: entriesRequiringLookup.map((entry) => {
			const fetcher = allowListFetchers[entry.type];
			return {
				queryKey: ["allow-list", entry.type, entry.id] as const,
				queryFn: async () => {
					if (!fetcher) {
						return entry.id;
					}
					return fetcher(entry.id);
				},
				staleTime: 5 * 60 * 1000,
			};
		}),
	});

	return useMemo(() => {
		const map = new Map<string, string>();
		uniqueEntries.forEach(({ key, type, id, displayName }) => {
			map.set(
				key,
				buildAllowListLabel(
					type,
					id,
					displayName ? { resolvedName: displayName } : undefined,
				),
			);
		});
		entriesRequiringLookup.forEach((entry, index) => {
			const result = lookupResults[index];
			if (result?.data) {
				map.set(
					entry.key,
					buildAllowListLabel(entry.type, entry.id, {
						resolvedName: result.data,
					}),
				);
			}
		});
		return map;
	}, [entriesRequiringLookup, lookupResults, uniqueEntries]);
};

const getAllowListLabels = (
	entries: readonly APIAllowListTarget[] | null | undefined,
	labelMap: Map<string, string>,
) => {
	if (!entries || entries.length === 0) {
		return ["Any resource"];
	}

	const normalized = entries
		.map(normalizeAllowListEntry)
		.filter((value) => value && value.length > 0);

	if (normalized.length === 0) {
		return ["*:*"];
	}

	const unique = Array.from(new Set(normalized));
	return unique.map((key) => labelMap.get(key) ?? defaultLabelForKey(key));
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
	children,
}) => {
	const allowListLabels = useAllowListLabelMap(tokens);

	return (
		<Stack>
			{Boolean(getTokensError) && <ErrorAlert error={getTokensError} />}
			{Boolean(deleteTokenError) && <ErrorAlert error={deleteTokenError} />}
			{children}

			<Table className="min-w-[1000px]">
				<TableHeader>
					<TableRow>
						<TableHead className="w-[12%]">ID</TableHead>
						<TableHead className="w-[14%]">Name</TableHead>
						<TableHead className="w-[18%]">Scopes</TableHead>
						<TableHead className="w-[24%]">Allow List</TableHead>
						<TableHead className="w-[8%]">Last Used</TableHead>
						<TableHead className="w-[8%]">Expires At</TableHead>
						<TableHead className="w-[8%]">Created At</TableHead>
						<TableHead className="w-[8%] text-right">Actions</TableHead>
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
								const scopeList = getScopeList(token);
								const allowListEntries = getAllowListLabels(
									token.allow_list,
									allowListLabels,
								);

								return (
									<TableRow
										key={token.id}
										data-testid={`token-${token.id}`}
										tabIndex={0}
									>
										<TableCell className="align-top">
											<div className="text-content-secondary break-all">
												{token.id}
											</div>
										</TableCell>

										<TableCell className="align-top">
											<div className="text-content-secondary break-words">
												{token.token_name}
											</div>
										</TableCell>

										<TableCell className="align-top">
											<div className="text-content-secondary break-words space-y-1">
												{scopeList.map((scope, index) => (
													<div key={`${scope}-${index}`}>{scope}</div>
												))}
											</div>
										</TableCell>

										<TableCell className="align-top">
											<div className="text-content-secondary break-words space-y-1">
												{allowListEntries.map((label, index) => (
													<div key={`${label}-${index}`}>{label}</div>
												))}
											</div>
										</TableCell>

										<TableCell className="align-top">
											{lastUsedOrNever(token.last_used)}
										</TableCell>

										<TableCell className="align-top">
											<span
												className="text-content-secondary"
												data-chromatic="ignore"
											>
												{dayjs(token.expires_at).fromNow()}
											</span>
										</TableCell>

										<TableCell className="align-top">
											<div className="text-content-secondary">
												{dayjs(token.created_at).fromNow()}
											</div>
										</TableCell>

										<TableCell className="align-top text-right">
											<Stack
												direction="row"
												spacing={1}
												alignItems="center"
												justifyContent="flex-end"
											>
												<Button
													asChild
													size="icon"
													variant="subtle"
													aria-label="Edit token"
													className="text-content-secondary hover:text-content-primary"
												>
													<RouterLink
														to={`${encodeURIComponent(token.token_name)}/edit`}
													>
														<PencilIcon
															aria-hidden="true"
															className="size-icon-sm"
														/>
														<span className="sr-only">Edit token</span>
													</RouterLink>
												</Button>

												<Button
													size="icon"
													variant="subtle"
													onClick={() => {
														onDelete(token);
													}}
													aria-label="Delete token"
													className="text-content-secondary hover:text-content-primary"
												>
													<TrashIcon
														aria-hidden="true"
														className="size-icon-sm"
													/>
													<span className="sr-only">Delete token</span>
												</Button>
											</Stack>
										</TableCell>
									</TableRow>
								);
							})}
						</Cond>
					</ChooseOne>
				</TableBody>
			</Table>
		</Stack>
	);
};
