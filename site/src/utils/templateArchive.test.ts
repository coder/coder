import { File as NodeFile } from "node:buffer";
import {
	createMockTemplateResourceArchive,
	MockTemplateResourceFiles,
} from "#/testHelpers/templateResources";
import {
	createFile,
	getFileText,
	moveFile,
	removeFile,
	updateFile,
} from "./filetree";
import { TarReader, TarWriter } from "./tar";
import { readTemplateArchive, writeTemplateArchive } from "./templateArchive";

beforeEach(() => vi.stubGlobal("File", NodeFile));
afterEach(() => vi.unstubAllGlobals());

it("round-trips nested modules while adding, deleting, renaming, and editing files", async () => {
	const initial = await readTemplateArchive(
		await createMockTemplateResourceArchive(),
	);
	let files = updateFile("main.tf", "# edited root", initial.files);
	files = createFile(
		"modules/coder-network-policy/outputs.tf",
		files,
		'output "id" { value = "example" }',
	);
	files = moveFile("README.md", "docs/README.md", files);
	files = removeFile("modules/coder-network-policy/variables.tf", files);
	const result = new TarReader();
	await result.readFile(await writeTemplateArchive(initial.archive, files));
	expect(result.getTextFile("main.tf")).toBe("# edited root");
	expect(result.getTextFile("modules/coder-network-policy/main.tf")).toBe(
		getFileText(
			"modules/coder-network-policy/main.tf",
			MockTemplateResourceFiles,
		),
	);
	expect(result.getTextFile("modules/coder-network-policy/outputs.tf")).toBe(
		'output "id" { value = "example" }',
	);
	expect(result.getTextFile("docs/README.md")).toBe(
		getFileText("README.md", MockTemplateResourceFiles),
	);
	expect(result.getTextFile("README.md")).toBeUndefined();
	expect(
		result.getTextFile("modules/coder-network-policy/variables.tf"),
	).toBeUndefined();
});

it("preserves binary bytes, file modes, and children preceding their directory header", async () => {
	const bytes = new Uint8Array([0, 255, 128, 42]);
	const writer = new TarWriter();
	writer.addFile("./modules/policy/blob.bin", bytes, { mode: 0o755 });
	writer.addFolder("./modules/");
	writer.addFolder("./");
	writer.addFile("./main.tf", "# root");
	await writer.write();
	const initial = await readTemplateArchive(writer.buffer);
	const result = new TarReader();
	await result.readFile(
		await writeTemplateArchive(
			initial.archive,
			updateFile("main.tf", "# changed", initial.files),
		),
	);
	const binary = result.getFileBlob("modules/policy/blob.bin");
	if (!binary) throw new Error("Binary file was lost from archive");
	expect(new Uint8Array(await binary.arrayBuffer())).toEqual(bytes);
	expect(
		result.fileInfo.find((file) => file.name === "modules/policy/blob.bin")
			?.mode,
	).toBe(0o755);
});
