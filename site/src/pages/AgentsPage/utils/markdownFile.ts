const MARKDOWN_EXTENSIONS = [".md", ".markdown"];

/**
 * Returns true when the file name's extension marks it as Markdown. The
 * check is case-insensitive and ignores directories in the path. MDX is
 * excluded because the shared renderer treats JSX as escaped text.
 */
export function isMarkdownFileName(fileName: string): boolean {
	const lower = fileName.toLowerCase();
	return MARKDOWN_EXTENSIONS.some((ext) => lower.endsWith(ext));
}
