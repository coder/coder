import { createContext, useContext, useEffect, useRef, useState } from "react";
import { useQueries, useQuery, useQueryClient } from "react-query";
import { API, type WorkspaceAgentFindFilesResult } from "#/api/api";
import {
	workspaceAgentDirectory,
	workspaceAgentDirectoryKey,
	workspaceAgentFindFiles,
} from "#/api/queries/workspaceFiles";
import {
	Command,
	CommandEmpty,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import {
	Popover,
	PopoverAnchor,
	PopoverContent,
} from "#/components/Popover/Popover";
import { useWorkspaceFiles } from "../../context/WorkspaceFilesContext";
import {
	findCandidateTokenStarts,
	findFilePathMentions,
	findQuotedPathEnd,
	findRejectedCandidateEnd,
	getFilePathCandidateStarts,
	parseFilePathMention,
	walkFilePath,
} from "../../utils/filePathMentions";
import { getPathBasename } from "../../utils/path";

// The agent's maximum; common names such as README.md can match many files.
const findFilesLimit = 200;

// The agent accepts at most this many roots per find-files request
// (maxFinderRoots in agent/agentfiles/findfiles.go).
const findFilesRootsPerRequest = 8;

type BareNameSearch = {
	results: WorkspaceAgentFindFilesResult[];
	/** Some request hit the result limit, so matches may be missing. */
	truncated: boolean;
};

/**
 * Searches every root for a bare file name, in requests of at most
 * `findFilesRootsPerRequest` roots, one after another so they do not
 * compete for the agent's index. A failed request counts as no matches,
 * so the result is cached like any other instead of retried per mention.
 */
const searchBareName = async (
	agentId: string,
	roots: readonly string[],
	name: string,
): Promise<BareNameSearch> => {
	const search: BareNameSearch = { results: [], truncated: false };
	for (let i = 0; i < roots.length; i += findFilesRootsPerRequest) {
		const batch = roots.slice(i, i + findFilesRootsPerRequest);
		try {
			const { results } = await API.findWorkspaceAgentFiles(
				agentId,
				batch,
				name,
				findFilesLimit,
			);
			search.results.push(...results);
			search.truncated ||= results.length >= findFilesLimit;
		} catch {
			// A failed batch has no matches.
		}
	}
	return search;
};

/**
 * Keeps the search results that a bare file name names. A file at a root
 * wins over files deeper in the tree, so README.md does not also offer
 * every docs/README.md.
 */
const pickMentionedFiles = (
	results: readonly WorkspaceAgentFindFilesResult[],
	name: string,
) => {
	const candidates = results.filter((result) => !result.is_dir);
	const atRoot = candidates.filter((result) => result.path === name);
	if (atRoot.length > 0) {
		return atRoot;
	}
	return candidates.filter((result) =>
		result.absolute_path.endsWith(`/${name}`),
	);
};

/** Where a verified mention leads. */
type MentionLink = {
	/**
	 * The verified absolute path, or the mentioned text when `choices`
	 * holds several matches.
	 */
	path: string;
	line?: number;
	/** Equally good matches in several roots; clicking offers a picker. */
	choices?: readonly WorkspaceAgentFindFilesResult[];
	/** The absolute `path` is a verified folder, shown in the Files panel. */
	isDirectory?: boolean;
	/** `choices` came from a search that hit its result limit. */
	truncated?: boolean;
};

const linkKey = (link: MentionLink | undefined) =>
	link === undefined
		? ""
		: JSON.stringify([
				link.path,
				link.line,
				link.isDirectory,
				link.truncated,
				link.choices?.map((choice) => choice.absolute_path),
			]);

/**
 * Mentioned text, and once `link` is set, a control that opens the file or
 * shows a folder in the Files panel. The text stays in the same element
 * whether or not it links, so verification does not replace the nodes a
 * reader has selected or focused.
 */
const FileMention: React.FC<{
	link: MentionLink | undefined;
	children: React.ReactNode;
}> = ({ link, children }) => {
	const files = useWorkspaceFiles();
	const [picking, setPicking] = useState(false);
	const controlRef = useRef<HTMLSpanElement>(null);
	const matches = link?.choices ?? [];
	const picker = matches.length > 1;

	const activate = () => {
		if (!files || !link) {
			return;
		}
		if (picker) {
			setPicking(true);
		} else if (link.isDirectory) {
			files.revealInFiles(link.path, { isDirectory: true });
		} else {
			files.openFile(link.path, { line: link.line });
		}
	};
	const openMatch = (match: WorkspaceAgentFindFilesResult) => {
		if (!files) {
			return;
		}
		if (match.is_dir) {
			files.revealInFiles(match.absolute_path, { isDirectory: true });
		} else {
			files.openFile(match.absolute_path, { line: link?.line });
		}
	};

	return (
		<Popover
			open={picking && picker}
			onOpenChange={(open) => {
				if (!open) {
					setPicking(false);
				}
			}}
		>
			<PopoverAnchor asChild>
				<span
					ref={controlRef}
					{...(link && {
						role: "button",
						tabIndex: 0,
						"aria-label": link.isDirectory
							? `Show ${link.path} in Files`
							: undefined,
						"aria-haspopup": picker ? ("dialog" as const) : undefined,
						"aria-expanded": picker ? picking : undefined,
						className:
							"cursor-pointer underline decoration-dotted decoration-content-secondary underline-offset-2 hover:decoration-content-link focus-visible:outline focus-visible:outline-2 focus-visible:outline-content-link",
						onClick: activate,
						onKeyDown: (event: React.KeyboardEvent) => {
							if (event.key === "Enter" || event.key === " ") {
								event.preventDefault();
								activate();
							}
						},
					})}
				>
					{children}
				</span>
			</PopoverAnchor>
			{picker && link && (
				<PopoverContent
					align="start"
					aria-label={`Files matching ${link.path}`}
					className="w-96 max-w-[90vw] p-0"
					onCloseAutoFocus={(event) => {
						event.preventDefault();
						controlRef.current?.focus();
					}}
				>
					<Command label={`Files matching ${link.path}`}>
						<CommandInput placeholder="Filter matching files" />
						<CommandList>
							<CommandEmpty>No matching files</CommandEmpty>
							{matches.map((match) => {
								const rootName = getPathBasename(match.root);
								return (
									<CommandItem
										key={match.absolute_path}
										value={match.absolute_path}
										aria-label={`${match.path} in ${rootName}`}
										className="items-baseline py-1.5"
										onSelect={() => {
											setPicking(false);
											openMatch(match);
										}}
									>
										<span className="min-w-0 truncate font-mono text-xs text-content-primary">
											{match.path}
										</span>
										<span className="ml-auto shrink-0 text-xs font-normal">
											{rootName}
										</span>
									</CommandItem>
								);
							})}
						</CommandList>
						<p className="m-0 border-0 border-t border-solid border-border px-3 py-1.5 text-xs text-content-secondary">
							{link.truncated
								? `Showing ${matches.length} matches from the first ${findFilesLimit} search results. Mention a longer path to narrow them down.`
								: `${matches.length} files match ${link.path}.`}
						</p>
					</Command>
				</PopoverContent>
			)}
		</Popover>
	);
};

/**
 * True while the enclosing message is still streaming. Candidates wait for
 * the final text before they list directories.
 */
export const FileMentionStreamingContext = createContext(false);

// Directory listings change rarely while a chat is read; reuse them across
// candidates instead of refetching per mention.
const directoryStaleTime = 60_000;

// Prose such as "and/or" makes candidates whose directories do not exist.
// Remembering the failure keeps every such candidate from listing again.
const missingDirectoryStaleTime = 30_000;

type FileMentionCandidateProps = {
	/** Text starting with a path-like token, possibly with spaces in it. */
	text: string;
	/**
	 * The verified path must cover the whole text, as in inline code or a
	 * mention in prose.
	 */
	whole: boolean;
	/** The character before the text; a quote bounds the path. */
	opener?: string;
	/** Today's rendering of the text, shown until a path is verified. */
	children: React.ReactNode;
};

/**
 * Text that may be a file path. While it is on screen and the message has
 * finished streaming, directory listings decide whether the path exists and
 * how far it extends, or a file search finds a bare file name; only the
 * verified part becomes a link.
 *
 * The walk is derived in render from the listings it needs, so it follows
 * listing refreshes and stops requesting as soon as it is not rendered.
 */
export const FileMentionCandidate: React.FC<FileMentionCandidateProps> = ({
	text,
	whole,
	opener,
	children,
}) => {
	const files = useWorkspaceFiles();
	const streaming = useContext(FileMentionStreamingContext);
	const queryClient = useQueryClient();
	const ref = useRef<HTMLSpanElement>(null);
	const [visible, setVisible] = useState(false);
	const [directories, setDirectories] = useState<readonly string[]>([]);
	const [lastLink, setLastLink] = useState<{
		text: string;
		key: string;
		link: MentionLink | undefined;
		end: number;
	}>({ text: "", key: "", link: undefined, end: 0 });
	const available = files !== undefined;

	useEffect(() => {
		const element = ref.current;
		if (!available || !element) {
			return;
		}
		const observer = new IntersectionObserver((entries) => {
			const latest = entries[entries.length - 1];
			if (latest) {
				setVisible(latest.isIntersecting);
			}
		});
		observer.observe(element);
		return () => {
			observer.disconnect();
			setVisible(false);
		};
	}, [available]);

	const agentId = files?.agentId ?? "";
	// Shared listings, observed without fetching, so a refresh from the file
	// tree or elsewhere changes the walk.
	const listings = useQueries({
		queries: directories.map((directory) => ({
			...workspaceAgentDirectory(agentId, directory),
			enabled: false,
		})),
	});
	const checks = useQueries({
		queries: directories.map((directory) => ({
			queryKey: [
				...workspaceAgentDirectoryKey(agentId, directory),
				"verifyPath",
			],
			queryFn: () =>
				queryClient
					.fetchQuery({
						...workspaceAgentDirectory(agentId, directory),
						staleTime: directoryStaleTime,
					})
					.then(
						(listing) => listing.contents,
						() => null,
					),
			enabled: available && visible && !streaming,
			staleTime: (query: { state: { data: unknown } }) =>
				query.state.data === null
					? missingDirectoryStaleTime
					: directoryStaleTime,
			retry: false,
		})),
	});

	const getEntries = (directory: string) => {
		const index = directories.indexOf(directory);
		if (index === -1) {
			return undefined;
		}
		return listings[index].data?.contents ?? checks[index].data;
	};
	// A bare file name such as FileMention.tsx can live anywhere in a
	// repository, so a search finds it instead of a walk from the roots.
	const mention = whole ? parseFilePathMention(text) : undefined;
	const bareName =
		mention && !mention.path.includes("/") ? mention.path : undefined;
	const searchRoots =
		files && files.repoRoots.length > 0 ? files.repoRoots : [""];
	const search = useQuery({
		queryKey: [
			...workspaceAgentFindFiles(
				agentId,
				searchRoots,
				bareName ?? "",
				findFilesLimit,
			).queryKey,
			"mention",
		],
		queryFn: () => searchBareName(agentId, searchRoots, bareName ?? ""),
		enabled: available && visible && !streaming && bareName !== undefined,
		staleTime: directoryStaleTime,
	});
	const starts =
		files && bareName === undefined
			? getFilePathCandidateStarts(
					text,
					files.repoRoots,
					files.defaultDirectory,
				)
			: [];
	const walks = starts.map((start) =>
		walkFilePath(start.root, text.slice(start.offset), getEntries),
	);
	const needed = [...new Set(walks.flatMap((walk) => walk.directories))];
	if (needed.join("\n") !== directories.join("\n")) {
		setDirectories(needed);
	}

	if (!files) {
		return children;
	}

	// What the lookups confirmed: the extent of the verified path and its
	// matches, or undefined while a lookup is still pending.
	let verification:
		| {
				end: number;
				line?: number;
				choices: WorkspaceAgentFindFilesResult[];
				truncated?: boolean;
		  }
		| undefined;
	if (bareName !== undefined) {
		if (search.data) {
			const matches = pickMentionedFiles(search.data.results, bareName);
			verification = {
				end: matches.length > 0 ? text.length : 0,
				line: mention?.line,
				choices: matches,
				truncated: search.data.truncated,
			};
		}
	} else if (walks.every((walk) => walk.verified !== undefined)) {
		// Equally long matches in different repositories are ambiguous, so
		// the mention offers all of them. Until every root has answered, a
		// slower root could still hold another match, so nothing links yet.
		// The default directory counts only when no repository matches.
		const quotedEnd = findQuotedPathEnd(text, opener);
		let end = 0;
		let line: number | undefined;
		let choices: WorkspaceAgentFindFilesResult[] = [];
		for (const [index, walk] of walks.entries()) {
			const verified = walk.verified;
			if (!verified || (starts[index].fallback && choices.length > 0)) {
				continue;
			}
			const walkEnd = starts[index].offset + verified.length;
			if (walkEnd < end || (quotedEnd !== -1 && walkEnd !== quotedEnd)) {
				continue;
			}
			if (walkEnd > end) {
				end = walkEnd;
				line = verified.line;
				choices = [];
			}
			if (
				!choices.some(
					(choice) => choice.absolute_path === verified.absolutePath,
				)
			) {
				const root = starts[index].root;
				choices.push({
					absolute_path: verified.absolutePath,
					path: verified.absolutePath.slice(root.replace(/\/$/, "").length + 1),
					root,
					is_dir: verified.isDirectory ?? false,
					score: 0,
				});
			}
		}
		verification = { end, line, choices };
	}

	const mentions = whole ? [] : findFilePathMentions(text);
	const leadEnd = mentions[0]?.start === 0 ? mentions[0].end : 0;
	// The verified path replaces the ordinary mention it starts with when it
	// is longer, or a folder of the same extent such as /etc/nginx/conf.d.
	// Otherwise that mention verifies itself and keeps its link's node.
	let link: MentionLink | undefined;
	let end = 0;
	if (
		verification &&
		verification.choices.length > 0 &&
		(whole
			? verification.end === text.length
			: verification.end > leadEnd ||
				(verification.end === leadEnd && verification.choices[0].is_dir))
	) {
		const { choices } = verification;
		end = verification.end;
		link = {
			path: choices.length > 1 ? text.slice(0, end) : choices[0].absolute_path,
			line: verification.line,
			choices: choices.length > 1 ? choices : undefined,
			isDirectory: choices.length === 1 && choices[0].is_dir,
			truncated: verification.truncated,
		};
	}

	// A new root set or a refetch makes the lookups pending again; the last
	// confirmed link stays until they answer, so the control a reader is
	// using does not disappear.
	const settled = verification !== undefined;
	const key = linkKey(link);
	if (settled && (lastLink.text !== text || lastLink.key !== key)) {
		setLastLink({ text, key, link, end });
	}
	if (!settled && lastLink.text === text) {
		link = lastLink.link;
		end = lastLink.end;
	}
	const known = settled || lastLink.text === text;

	if (whole) {
		return (
			<span ref={ref}>
				<FileMention link={link}>{children}</FileMention>
			</span>
		);
	}

	// The text renders from the same keyed segments before and after
	// verification, so links and text nodes that do not change keep their
	// DOM nodes, focus, and selection. A later path-like token on the line
	// may start another spaced path, so once this walk has settled, and the
	// extent of this path no longer moves, the rest of the text from that
	// token is its own candidate. That token must lie past this path, or
	// past where a rejected one could have ended.
	const segments: React.ReactNode[] = [];
	let offset = 0;
	if (link) {
		segments.push(
			<FileMention key="lead" link={link}>
				{text.slice(0, end)}
			</FileMention>,
		);
		offset = end;
	}
	const confirmed = (verification?.choices.length ?? 0) > 0;
	const nextFrom =
		confirmed || link
			? Math.max(offset, verification?.end ?? end)
			: findRejectedCandidateEnd(text);
	const next = known
		? findCandidateTokenStarts(text).find(
				(start) => start > 0 && start >= nextFrom,
			)
		: undefined;
	const segmentsEnd = next ?? text.length;
	for (const mention of mentions) {
		if (mention.start < offset || mention.start >= segmentsEnd) {
			continue;
		}
		if (mention.start > offset) {
			segments.push(text.slice(offset, mention.start));
		}
		const mentionText = text.slice(mention.start, mention.end);
		segments.push(
			<FileMentionCandidate
				key={mention.start === 0 ? "lead" : `mention-${mention.start}`}
				text={mentionText}
				whole
			>
				{mentionText}
			</FileMentionCandidate>,
		);
		offset = mention.end;
	}
	if (offset < segmentsEnd) {
		segments.push(text.slice(offset, segmentsEnd));
	}
	if (next !== undefined) {
		const rest = text.slice(next);
		segments.push(
			<FileMentionCandidate
				key={`candidate-${next}`}
				text={rest}
				whole={false}
				opener={text[next - 1]}
			>
				{rest}
			</FileMentionCandidate>,
		);
	}
	return <span ref={ref}>{segments}</span>;
};
