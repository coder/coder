import { useTheme } from "@emotion/react";
import { SyntaxHighlighter } from "components/SyntaxHighlighter/SyntaxHighlighter";
import set from "lodash/set";
import { EditIcon } from "lucide-react";
import { linkToTemplate, useLinks } from "modules/navigation";
import { type FC, useCallback, useMemo } from "react";
import { Link } from "react-router";
import { cn } from "utils/cn";
import type { FileTree } from "utils/filetree";
import type { TemplateVersionFiles } from "utils/templateVersion";
import { TemplateFileTree } from "./TemplateFileTree";

interface TemplateFilesProps {
	organizationName: string;
	templateName: string;
	versionName: string;
	currentFiles: TemplateVersionFiles;
	/**
	 * Files used to compare with current files
	 */
	baseFiles?: TemplateVersionFiles;
}

export const TemplateFiles: FC<TemplateFilesProps> = ({
	organizationName,
	templateName,
	versionName,
	currentFiles,
	baseFiles,
}) => {
	const getLink = useLinks();
	const theme = useTheme();

	const fileInfo = useCallback(
		(filename: string) => {
			const value = currentFiles[filename].trim();
			const previousValue = baseFiles ? baseFiles[filename]?.trim() : undefined;
			const hasDiff = previousValue && value !== previousValue;

			return {
				value,
				previousValue,
				hasDiff,
			};
		},
		[baseFiles, currentFiles],
	);

	const fileTree: FileTree = useMemo(() => {
		const tree: FileTree = {};
		for (const filename of Object.keys(currentFiles)) {
			const info = fileInfo(filename);
			set(tree, filename.split("/"), info.value);
		}
		return tree;
	}, [fileInfo, currentFiles]);

	const versionLink = `${getLink(
		linkToTemplate(organizationName, templateName),
	)}/versions/${versionName}`;

	return (
		<div>
			<div className="flex items-start gap-8">
				<div className={classNames.sidebar}>
					<TemplateFileTree
						fileTree={fileTree}
						onSelect={(path: string) => {
							window.location.hash = path;
						}}
						Label={({ path, filename, isFolder }) => {
							if (isFolder) {
								return <>{filename}</>;
							}

							const hasDiff = fileInfo(path).hasDiff;
							return (
								<span
									css={hasDiff && { color: theme.roles.warning.fill.outline }}
								>
									{filename}
								</span>
							);
						}}
					/>
				</div>

				<div className={classNames.files} data-testid="template-files-content">
					{Object.keys(currentFiles)
						.sort((a, b) => a.localeCompare(b))
						.map((filename) => {
							const info = fileInfo(filename);

							return (
								<div
									key={filename}
									className={classNames.filePanel}
									id={filename}
								>
									<header className={classNames.fileHeader}>
										<span
											className={cn({
												"text-content-warning": info.hasDiff,
											})}
										>
											{filename}
										</span>

										<div className="ml-auto">
											<Link
												to={`${versionLink}/edit?path=${filename}`}
												className={cn([
													"flex gap-1 items-center text-sm leading-none no-underline",
													"text-content-secondary hover:text-content-primary",
												])}
											>
												<EditIcon className="text-inherit size-icon-xs" />
												Edit
											</Link>
										</div>
									</header>
									<SyntaxHighlighter
										language={getLanguage(filename)}
										value={info.value}
										compareWith={info.previousValue}
										editorProps={{
											// 18 is the editor line height
											height: Math.min(numberOfLines(info.value) * 18, 560),
											onMount: (editor) => {
												editor.updateOptions({
													scrollBeyondLastLine: false,
												});
											},
										}}
									/>
								</div>
							);
						})}
				</div>
			</div>
		</div>
	);
};

const languageByExtension: Record<string, string> = {
	tf: "hcl",
	hcl: "hcl",
	md: "markdown",
	mkd: "markdown",
	sh: "shell",
	tpl: "tpl",
	protobuf: "protobuf",
	nix: "dockerfile",
	json: "json",
};

const getLanguage = (filename: string) => {
	// Dockerfile can be like images/Dockerfile or Dockerfile.java
	if (filename.includes("Dockerfile")) {
		return "dockerfile";
	}
	const extension = filename.split(".").pop();
	return languageByExtension[extension ?? ""];
};

const numberOfLines = (content: string) => {
	return content.split("\n").length;
};

const classNames = {
	sidebar:
		"w-60 flex-shrink-0 rounded-lg overflow-auto border border-solid border-zinc-700 py-1 sticky top-8",
	files: "flex flex-col gap-4 flex-1",
	filePanel: "rounded-lg border border-solid border-zinc-700 overflow-hidden",
	fileHeader:
		"py-2 px-4 flex gap-2 items-center border-0 border-b border-solid border-zinc-700 text-[13px] font-medium",
};
