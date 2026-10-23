import { getPathBasename } from "../../utils/path";

// Common languages under the names Monaco 0.55 gives them (each language's
// first alias), with the file extensions and, without a leading dot, the
// file names Monaco registers for them. The file header names the language
// from these before the editor's chunk loads, and from Monaco for others.
const languageFiles: Readonly<Record<string, readonly string[]>> = {
	Batch: [".bat", ".cmd"],
	C: [".c", ".h"],
	"C#": [".cs"],
	"C++": [".cpp", ".cc", ".cxx", ".hpp", ".hh", ".hxx"],
	CSS: [".css"],
	Dart: [".dart"],
	Dockerfile: ["dockerfile", ".dockerfile"],
	Go: [".go"],
	GraphQL: [".graphql", ".gql"],
	HTML: [".html", ".htm"],
	Ini: [".ini", ".properties", ".gitconfig", ".editorconfig", "config"],
	Java: [".java"],
	JavaScript: [".js", ".jsx", ".mjs", ".cjs"],
	JSON: [".json"],
	Kotlin: [".kt", ".kts"],
	Less: [".less"],
	Lua: [".lua"],
	Markdown: [".md", ".markdown"],
	MDX: [".mdx"],
	Perl: [".pl", ".pm"],
	PHP: [".php"],
	"Plain Text": [".txt"],
	PowerShell: [".ps1", ".psm1", ".psd1"],
	protobuf: [".proto"],
	Python: [".py", ".pyw"],
	Ruby: [".rb", ".gemspec", "gemfile", "rakefile"],
	Rust: [".rs"],
	Sass: [".scss"],
	Scala: [".scala", ".sc", ".sbt"],
	Shell: [".sh", ".bash"],
	SQL: [".sql"],
	Swift: [".swift"],
	Terraform: [".tf", ".tfvars", ".hcl"],
	TypeScript: [".ts", ".tsx", ".cts", ".mts"],
	XML: [".xml", ".xsd", ".svg", ".csproj"],
	YAML: [".yaml", ".yml"],
};

const languagesByFile = new Map(
	Object.entries(languageFiles).flatMap(([language, files]) =>
		files.map((file) => [file, language] as const),
	),
);

/**
 * The display name of the language of the file at `path`, such as "Go",
 * matched like Monaco does: the whole file name, then the longest extension,
 * ignoring case. Undefined for a file this map does not know.
 */
export const getFileLanguage = (path: string): string | undefined => {
	const name = getPathBasename(path).toLowerCase();
	const language = languagesByFile.get(name);
	if (language) {
		return language;
	}
	for (
		let dot = name.indexOf(".");
		dot !== -1;
		dot = name.indexOf(".", dot + 1)
	) {
		const extensionLanguage = languagesByFile.get(name.slice(dot));
		if (extensionLanguage) {
			return extensionLanguage;
		}
	}
	return undefined;
};
