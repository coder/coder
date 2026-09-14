import { TarReader, TarWriter } from "./tar";
import { createTemplateVersionFileTree } from "./templateVersion";

test("createTemplateVersionFileTree ignores empty path segments", async () => {
	const writer = new TarWriter();
	writer.addFolder("files/etc/apt/");
	writer.addFile(
		"files/etc/apt/sources.list",
		"deb http://example.com stable main",
	);

	const tarFile = await writer.write();
	const reader = new TarReader();
	await reader.readFile(tarFile);

	expect(createTemplateVersionFileTree(reader)).toEqual({
		files: {
			etc: {
				apt: {
					"sources.list": "deb http://example.com stable main",
				},
			},
		},
	});
});
