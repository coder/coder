import { SyntaxHighlighter } from "components/SyntaxHighlighter/SyntaxHighlighter";
import set from "lodash/set";
import { EditIcon } from "lucide-react";
import { linkToTemplate, useLinks } from "modules/navigation";
import { type FC, useCallback, useMemo } from "react";
import { Link as RouterLink } from "react-router";
import { cn } from "utils/cn";
import type { FileTree } from "utils/filetree";
import type { TemplateVersionFiles } from "utils/templateVersion";
import { getTemplateFileIcon } from "./TemplateFileIcon";
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
				<div className="sticky top-8 w-[240px] shrink-0 overflow-auto rounded-lg border border-solid border-surface-quaternary py-1">
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
								<span className={cn(hasDiff && "text-content-warning")}>
									{filename}
								</span>
							);
						}}
					/>
				</div>

				<div
					className="flex flex-1 flex-col gap-4"
					data-testid="template-files-content"
				>
					{Object.keys(currentFiles)
						.sort((a, b) => a.localeCompare(b))
						.map((filename) => {
							const TemplateFileIcon = getTemplateFileIcon(filename, false);
							const info = fileInfo(filename);

							return (
								<div
									key={filename}
									id={filename}
									className="overflow-hidden rounded-lg border border-solid border-surface-quaternary"
								>
									<header className="flex items-center gap-2 border-0 border-b border-solid border-surface-quaternary px-4 py-2 text-[13px] font-medium">
										<div className="flex items-center gap-2">
											<TemplateFileIcon className="text-content-secondary size-icon-xs" />
											<span
												className={cn({
													"text-content-warning": info.hasDiff,
												})}
											>
												{filename}
											</span>
										</div>

										<div className="ml-auto">
											<RouterLink
												to={`${versionLink}/edit?path=${filename}`}
												className="flex items-center gap-1 text-sm no-underline text-content-secondary hover:text-content-primary"
											>
												<EditIcon className="text-inherit size-icon-xs" />
												Edit
											</RouterLink>
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
