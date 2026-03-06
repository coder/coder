import { chatDiffFileContent } from "api/queries/chats";
import { FileIcon } from "components/FileIcon/FileIcon";
import { ImageOff, Loader2Icon } from "lucide-react";
import { type FC, useEffect } from "react";
import { useQuery } from "react-query";
import { cn } from "utils/cn";

// -------------------------------------------------------------------
// Types
// -------------------------------------------------------------------

export interface BinaryImageDiff {
	name: string;
	changeType: "new" | "deleted" | "change";
	/** Literal discriminator for identifying binary image diffs. */
	isBinaryImage: true;
}

// -------------------------------------------------------------------
// Constants
// -------------------------------------------------------------------

/** Matches common image file extensions at the end of a path. */
const IMAGE_EXTENSIONS = /\.(png|jpe?g|gif|svg|webp|bmp|ico|avif)$/i;

/** Maps a change type to a human-readable label for accessibility. */
const CHANGE_TYPE_LABELS: Record<BinaryImageDiff["changeType"], string> = {
	new: "Added",
	deleted: "Deleted",
	change: "Modified",
};

// -------------------------------------------------------------------
// Parsing
// -------------------------------------------------------------------

/**
 * Parses a raw unified diff string to extract entries for binary
 * image files. Git represents binary files differently from text
 * files — instead of line-by-line hunks, they appear as:
 *
 *   Binary files /dev/null and b/path/to/image.png differ
 *
 * This function splits on `diff --git ` boundaries, identifies
 * blocks that contain "Binary files", extracts the file path from
 * the header, and filters to only image extensions.
 */
export function parseBinaryImageDiffs(rawDiff: string): BinaryImageDiff[] {
	// Split on diff boundaries. The first element is empty or
	// preamble text before the first diff block, so we skip it.
	const blocks = rawDiff.split("diff --git ").slice(1);
	const results: BinaryImageDiff[] = [];

	for (const block of blocks) {
		if (!block.includes("Binary files")) {
			continue;
		}

		// Extract file name from the `a/path b/path` header line.
		// The header is the first line of the block, formatted as:
		//   a/some/file.png b/some/file.png
		const headerMatch = block.match(/^a\/(.+?)\s+b\/(.+)/m);
		if (!headerMatch) {
			continue;
		}

		const filePath = headerMatch[2];
		if (!IMAGE_EXTENSIONS.test(filePath)) {
			continue;
		}

		// Determine the change type from mode markers.
		let changeType: BinaryImageDiff["changeType"] = "change";
		if (block.includes("new file mode")) {
			changeType = "new";
		} else if (block.includes("deleted file mode")) {
			changeType = "deleted";
		}

		results.push({
			name: filePath,
			changeType,
			isBinaryImage: true,
		});
	}

	return results;
}

// -------------------------------------------------------------------
// Checkerboard background styles
// -------------------------------------------------------------------

/**
 * Tailwind classes for a checkerboard pattern backdrop. Uses CSS
 * custom properties so that `dark:` variants handle the theme
 * switch automatically without needing an `isDark` prop.
 */
const CHECKERBOARD_CLASSES = [
	// Light-mode checkerboard color.
	"[--checker-color:#f0f0f0]",
	// Dark-mode override via Tailwind's dark variant.
	"dark:[--checker-color:#2a2a2a]",
	// The four-gradient checkerboard pattern.
	"[background-image:linear-gradient(45deg,var(--checker-color)_25%,transparent_25%),linear-gradient(-45deg,var(--checker-color)_25%,transparent_25%),linear-gradient(45deg,transparent_75%,var(--checker-color)_75%),linear-gradient(-45deg,transparent_75%,var(--checker-color)_75%)]",
	"[background-size:16px_16px]",
	"[background-position:0_0,0_8px,8px_-8px,-8px_0px]",
].join(" ");

// -------------------------------------------------------------------
// Change type styling helpers
// -------------------------------------------------------------------

/** Maps a change type to a Tailwind text-color class for badges. */
export function badgeColor(type: BinaryImageDiff["changeType"]): string {
	switch (type) {
		case "new":
			return "text-green-700 dark:text-green-300";
		case "deleted":
			return "text-red-700 dark:text-red-300";
		case "change":
			return "text-orange-700 dark:text-orange-300";
	}
}

/** Short letter shown after the filename, matching VS Code style. */
export function badgeLetter(type: BinaryImageDiff["changeType"]): string {
	switch (type) {
		case "new":
			return "A";
		case "deleted":
			return "D";
		case "change":
			return "M";
	}
}

// -------------------------------------------------------------------
// Internal sub-components
// -------------------------------------------------------------------

/**
 * Fetches image content through the backend proxy and renders it.
 * The proxy handles GitHub authentication so this works for both
 * public and private repositories.
 */
const ProxiedDiffImage: FC<{
	chatId: string;
	filePath: string;
	gitRef: string;
	alt: string;
}> = ({ chatId, filePath, gitRef, alt }) => {
	const {
		data: imageUrl,
		isLoading,
		isError,
	} = useQuery({
		...chatDiffFileContent(chatId, filePath, gitRef),
		// Keep the image URL cached for the session to avoid
		// re-fetching images the user has already seen.
		staleTime: Number.POSITIVE_INFINITY,
	});

	// Revoke the object URL when the component unmounts or the
	// URL changes to prevent browser memory leaks.
	useEffect(() => {
		return () => {
			if (imageUrl) {
				URL.revokeObjectURL(imageUrl);
			}
		};
	}, [imageUrl]);

	if (isLoading) {
		return (
			<div
				className="flex items-center justify-center p-8 text-content-secondary"
				role="status"
			>
				<Loader2Icon
					className="size-6 animate-spin opacity-50"
					aria-hidden="true"
				/>
				<span className="sr-only">Loading image…</span>
			</div>
		);
	}

	if (isError || !imageUrl) {
		return (
			<div
				className="flex flex-col items-center justify-center gap-2 p-8 text-content-secondary"
				role="alert"
			>
				<ImageOff className="size-8 opacity-50" aria-hidden="true" />
				<span className="text-xs">Failed to load image</span>
			</div>
		);
	}

	return (
		<div
			className={cn(
				"flex items-center justify-center rounded p-2",
				CHECKERBOARD_CLASSES,
			)}
		>
			<img
				src={imageUrl}
				alt={alt}
				className="max-h-[400px] max-w-full object-contain"
			/>
		</div>
	);
};

/**
 * Placeholder shown when a branch ref is not available and
 * the image cannot be fetched.
 */
const NoBranchPlaceholder: FC<{ action: string }> = ({ action }) => (
	<div className="flex items-center justify-center p-8 text-xs text-content-secondary">
		Image {action} — preview unavailable without branch info.
	</div>
);

// -------------------------------------------------------------------
// Main component
// -------------------------------------------------------------------

interface ImageDiffViewProps {
	diff: BinaryImageDiff;
	chatId: string;
	/** The current feature branch (head). */
	branch?: string;
	/** The base branch to compare against for deletions/modifications. */
	baseBranch?: string;
}

/**
 * Renders a visual diff for binary image files, similar to GitHub's
 * image diff UI. Supports added, deleted, and modified images with
 * appropriate visual treatment for each change type.
 *
 * Images are fetched through the backend proxy endpoint which
 * handles GitHub authentication, so this works for both public
 * and private repositories.
 */
export const ImageDiffView: FC<ImageDiffViewProps> = ({
	diff,
	chatId,
	branch,
	baseBranch,
}) => {
	const fileName = diff.name.split("/").pop() ?? diff.name;
	const baseRef = baseBranch ?? "main";

	return (
		<div className="border-b border-solid border-border-default">
			{/* Sticky file header — matches the existing diff panel style. */}
			<div
				className="sticky top-0 z-10 flex items-center gap-2 border-b border-solid border-border-default bg-surface-secondary px-3 py-1.5"
				style={{ fontSize: 13 }}
			>
				<FileIcon fileName={fileName} className="shrink-0" />
				<span className="min-w-0 truncate text-content-primary">
					{diff.name}
				</span>
				<span
					role="img"
					className={cn(
						"ml-auto shrink-0 text-xs font-medium",
						badgeColor(diff.changeType),
					)}
					title={CHANGE_TYPE_LABELS[diff.changeType]}
					aria-label={CHANGE_TYPE_LABELS[diff.changeType]}
				>
					{" "}
					{badgeLetter(diff.changeType)}
				</span>
			</div>

			{/* Image content area */}
			<div className="p-4">
				{diff.changeType === "new" && (
					<div className="rounded border-l-4 border-solid border-green-600 pl-4">
						<span className="mb-2 inline-block rounded bg-green-100 px-2 py-0.5 text-xs font-medium text-green-800 dark:bg-green-950 dark:text-green-300">
							Added
						</span>
						{branch ? (
							<ProxiedDiffImage
								chatId={chatId}
								filePath={diff.name}
								gitRef={branch}
								alt={`Added: ${diff.name}`}
							/>
						) : (
							<NoBranchPlaceholder action="added" />
						)}
					</div>
				)}

				{diff.changeType === "deleted" && (
					<div className="rounded border-l-4 border-solid border-red-600 pl-4">
						<span className="mb-2 inline-block rounded bg-red-100 px-2 py-0.5 text-xs font-medium text-red-800 dark:bg-red-950 dark:text-red-300">
							Removed
						</span>
						<ProxiedDiffImage
							chatId={chatId}
							filePath={diff.name}
							gitRef={baseRef}
							alt={`Removed: ${diff.name}`}
						/>
					</div>
				)}

				{diff.changeType === "change" && (
					<div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
						<div>
							<span className="mb-2 block text-xs font-medium text-content-secondary">
								Before
							</span>
							<ProxiedDiffImage
								chatId={chatId}
								filePath={diff.name}
								gitRef={baseRef}
								alt={`Before: ${diff.name}`}
							/>
						</div>
						<div>
							<span className="mb-2 block text-xs font-medium text-content-secondary">
								After
							</span>
							{branch ? (
								<ProxiedDiffImage
									chatId={chatId}
									filePath={diff.name}
									gitRef={branch}
									alt={`After: ${diff.name}`}
								/>
							) : (
								<NoBranchPlaceholder action="modified" />
							)}
						</div>
					</div>
				)}
			</div>
		</div>
	);
};
