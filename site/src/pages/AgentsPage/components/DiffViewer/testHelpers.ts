export function generateLargeDiff(
	fileCount: number,
	linesPerFile: number,
): string {
	const dirs = ["src", "lib", "utils", "components", "hooks"];
	const files: string[] = [];
	for (let f = 0; f < fileCount; f++) {
		const dir = dirs[f % dirs.length];
		const fileName = `${dir}/module${f}.ts`;
		const deletions = Math.floor(linesPerFile / 10);
		const additionsReplace = Math.floor(linesPerFile / 10);
		const additionsNew = Math.floor(linesPerFile / 25);
		const oldCount = linesPerFile + deletions;
		const newCount = linesPerFile + additionsReplace + additionsNew;
		const lines = [
			`diff --git a/${fileName} b/${fileName}`,
			`index ${f.toString(16).padStart(7, "0")}..${(f + 1).toString(16).padStart(7, "0")} 100644`,
			`--- a/${fileName}`,
			`+++ b/${fileName}`,
			`@@ -1,${oldCount} +1,${newCount} @@`,
		];
		for (let i = 1; i <= linesPerFile; i++) {
			lines.push(` // context line ${i} in ${fileName}`);
			if (i % 10 === 0) {
				lines.push(`-  const old${i} = getValue(${i});`);
				lines.push(`+  const new${i} = getUpdatedValue(${i});`);
			}
			if (i % 25 === 0) {
				lines.push(`+  // Added: validation for ${fileName} at line ${i}`);
			}
		}
		files.push(lines.join("\n"));
	}
	return files.join("\n");
}
