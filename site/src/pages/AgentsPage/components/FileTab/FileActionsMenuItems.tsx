import { ExternalLinkIcon } from "lucide-react";
import { useQuery } from "react-query";
import type {
	ContextMenuItem,
	ContextMenuSeparator,
} from "#/components/ContextMenu/ContextMenu";
import type {
	DropdownMenuItem,
	DropdownMenuSeparator,
} from "#/components/DropdownMenu/DropdownMenu";
import { useClipboard } from "#/hooks/useClipboard";
import { useWorkspaceFiles } from "../../context/WorkspaceFilesContext";
import {
	changeRequestLabel,
	getChangeRequestFilesUrl,
	getFileDiffAnchor,
	getFileWebLinks,
	gitProviderNames,
} from "../../utils/gitWebUrl";
import { findRepoRoot } from "../MainTabs/useFileTabRefresh";

type FileActionsMenuItemsProps = {
	readonly Item: typeof DropdownMenuItem | typeof ContextMenuItem;
	readonly Separator:
		| typeof DropdownMenuSeparator
		| typeof ContextMenuSeparator;
	/** The file's absolute path. */
	readonly path: string;
	/** The editor's cursor line, which links to the file scroll to. */
	readonly line?: number;
};

/** Actions on an open file, shared by its header menu and its tab's menu. */
export const FileActionsMenuItems: React.FC<FileActionsMenuItemsProps> = ({
	Item,
	Separator,
	path,
	line,
}) => {
	const workspaceFiles = useWorkspaceFiles();
	const { copyToClipboard } = useClipboard();
	const root = workspaceFiles && findRepoRoot(workspaceFiles.repoRoots, path);
	const relativePath =
		root && root !== path ? path.slice(root.length + 1) : undefined;
	const repository = root ? workspaceFiles?.repositories.get(root) : undefined;
	const links =
		workspaceFiles && relativePath && repository?.remote_origin
			? getFileWebLinks({
					origin: repository.remote_origin,
					branch: repository.branch,
					headCommit: repository.head_commit ?? "",
					relativePath,
					line,
					diffStatuses: workspaceFiles.diffStatuses,
				})
			: undefined;
	const provider = links?.repository.provider;
	// Hashing is asynchronous, so the pull request link gains its file's
	// anchor a moment after the menu opens.
	const anchorQuery = useQuery({
		queryKey: ["fileDiffAnchor", provider, relativePath],
		queryFn: async () =>
			(provider &&
				relativePath &&
				(await getFileDiffAnchor(provider, relativePath))) ||
			null,
		enabled: Boolean(links?.changeRequest),
		staleTime: Number.POSITIVE_INFINITY,
	});

	return (
		<>
			<Item onSelect={() => void copyToClipboard(path)}>Copy path</Item>
			{relativePath !== undefined && (
				<Item onSelect={() => void copyToClipboard(relativePath)}>
					Copy relative path
				</Item>
			)}
			{workspaceFiles && (
				<Item onSelect={() => workspaceFiles.revealInFiles(path)}>
					Reveal in Files
				</Item>
			)}
			{provider && (links?.blob || links?.changeRequest) && <Separator />}
			{provider && links?.blob && (
				<Item asChild>
					<a
						href={links.blob.url}
						target="_blank"
						rel="noreferrer"
						title={
							links.blob.commit &&
							`Opens commit ${links.blob.commit.slice(0, 7)}, the checked-out HEAD. The page is missing until that commit is pushed.`
						}
					>
						Open on {gitProviderNames[provider]}
						<ExternalLinkIcon className="ml-auto size-3.5!" />
					</a>
				</Item>
			)}
			{provider && links?.changeRequest && (
				<Item asChild>
					<a
						href={getChangeRequestFilesUrl(
							provider,
							links.changeRequest.url,
							anchorQuery.data ?? undefined,
						)}
						target="_blank"
						rel="noreferrer"
					>
						Open in {changeRequestLabel(provider, links.changeRequest.number)}
						<ExternalLinkIcon className="ml-auto size-3.5!" />
					</a>
				</Item>
			)}
		</>
	);
};
