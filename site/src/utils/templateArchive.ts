import { type FileTree, traverse } from "./filetree";
import { TarReader, TarWriter } from "./tar";
import { createTemplateVersionFileTree } from "./templateVersion";

/** Retains the original archive so untouched binary files and modes survive builds. */
export const readTemplateArchive = async (data: ArrayBuffer) => {
	const archive = new TarReader();
	await archive.readFile(data);
	return { archive, files: createTemplateVersionFileTree(archive) };
};

/** Serializes edits without converting untouched archive entries to UTF-8. */
export const writeTemplateArchive = async (
	archive: TarReader,
	files: FileTree,
) => {
	const writer = new TarWriter();
	traverse(files, (content, _name, path) => {
		const original = archive.fileInfo.find(
			(entry) =>
				entry.name
					.split("/")
					.filter((part) => part !== "" && part !== ".")
					.join("/") === path,
		);
		if (typeof content === "string") {
			const unchanged =
				original && archive.getTextFile(original.name) === content;
			writer.addFile(
				path,
				unchanged ? (archive.getFileBlob(original.name) ?? content) : content,
				original,
			);
		} else {
			writer.addFolder(path, original);
		}
	});
	const blob = await writer.write();
	return new File([blob], "template.tar", { type: "application/x-tar" });
};
