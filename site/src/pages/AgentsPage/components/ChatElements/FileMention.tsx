import { createContext, useContext, useEffect, useRef, useState } from "react";
import { useQueries, useQueryClient } from "react-query";
import type { WorkspaceAgentFindFilesResult } from "#/api/api";
import {
	workspaceAgentDirectory,
	workspaceAgentDirectoryKey,
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
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { useWorkspaceFiles } from "../../context/WorkspaceFilesContext";
import {
	findCandidateTokenStarts,
	findFilePathMentions,
	findQuotedPathEnd,
	findRejectedCandidateEnd,
	getFilePathCandidateStarts,
	walkFilePath,
} from "../../utils/filePathMentions";
import { getPathBasename } from "../../utils/path";

type FileMentionProps = {
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
	children: React.ReactNode;
};

/**
 * A path in chat text that listings confirmed. Clicking opens the file, or
 * shows a folder in the Files panel.
 */
export const FileMention: React.FC<FileMentionProps> = ({
	path,
	line,
	choices,
	isDirectory = false,
	children,
}) => {
	const files = useWorkspaceFiles();
	const [picking, setPicking] = useState(false);

	if (!files) {
		return children;
	}

	const openMatch = (match: WorkspaceAgentFindFilesResult) => {
		if (match.is_dir) {
			files.revealInFiles(match.absolute_path, { isDirectory: true });
		} else {
			files.openFile(match.absolute_path, { line });
		}
	};

	const matches = choices ?? [];
	return (
		<Popover
			open={picking && matches.length > 1}
			onOpenChange={(open) => {
				if (!open) {
					setPicking(false);
				}
			}}
		>
			<PopoverTrigger asChild>
				<button
					type="button"
					aria-label={isDirectory ? `Show ${path} in Files` : undefined}
					className="inline cursor-pointer border-0 bg-transparent p-0 text-left font-[inherit] text-inherit underline decoration-dotted decoration-content-secondary underline-offset-2 hover:decoration-content-link"
					onClick={(event) => {
						// The trigger's own toggle would also close the picker.
						event.preventDefault();
						if (matches.length > 1) {
							setPicking(true);
						} else if (isDirectory) {
							files.revealInFiles(path, { isDirectory: true });
						} else {
							files.openFile(path, { line });
						}
					}}
				>
					{children}
				</button>
			</PopoverTrigger>
			<PopoverContent
				align="start"
				aria-label={`Files matching ${path}`}
				className="w-96 max-w-[90vw] p-0"
			>
				<Command label={`Files matching ${path}`}>
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
						{`${matches.length} files match ${path}.`}
					</p>
				</Command>
			</PopoverContent>
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
 * how far it extends; only the verified part becomes a link.
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
	const starts = files
		? getFilePathCandidateStarts(text, files.repoRoots, files.defaultDirectory)
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

	// Equally long matches in different repositories are ambiguous, so the
	// mention offers all of them. Until every root has answered, a slower
	// root could still hold another match, so nothing links yet. The default
	// directory counts only when no repository matches.
	const settled = walks.every((walk) => walk.verified !== undefined);
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
			!choices.some((choice) => choice.absolute_path === verified.absolutePath)
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
	const mentions = whole ? [] : findFilePathMentions(text);
	const leadEnd = mentions[0]?.start === 0 ? mentions[0].end : 0;
	// The verified path links when it covers at least the ordinary mention
	// it starts with; a shorter one leaves that mention to verify itself.
	const extended =
		settled &&
		choices.length > 0 &&
		(whole ? end === text.length : end >= leadEnd);
	const lead = extended && (
		<FileMention
			key="lead"
			path={choices.length > 1 ? text.slice(0, end) : choices[0].absolute_path}
			line={line}
			choices={choices.length > 1 ? choices : undefined}
			isDirectory={choices.length === 1 && choices[0].is_dir}
		>
			{whole ? children : text.slice(0, end)}
		</FileMention>
	);

	if (whole) {
		return <span ref={ref}>{lead || children}</span>;
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
	if (lead) {
		segments.push(lead);
		offset = end;
	}
	const nextFrom =
		choices.length > 0 ? Math.max(offset, end) : findRejectedCandidateEnd(text);
	const next = settled
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
