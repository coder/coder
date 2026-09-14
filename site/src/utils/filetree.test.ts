import {
	createFile,
	existsFile,
	type FileTree,
	getFileContent,
	isFolder,
	moveFile,
	removeFile,
	traverse,
} from "./filetree";

test("createFile() set file into the file tree", () => {
	const fileTree: FileTree = {
		"main.tf": "terraform",
		images: { "java.Dockerfile": "java dockerfile" },
	};
	const updatedFileTree = createFile(
		"images/python.Dockerfile",
		fileTree,
		"python dockerfile",
	);
	expect((updatedFileTree.images as FileTree)["python.Dockerfile"]).toEqual(
		"python dockerfile",
	);
	// Verify the original FileTree was not modified.
	expect((fileTree.images as FileTree)["python.Dockerfile"]).toBeUndefined();
});

test("getFileContent() return the file content from the file tree", () => {
	const fileTree: FileTree = {
		"main.tf": "terraform content",
		images: { "java.Dockerfile": "java dockerfile" },
	};
	expect(getFileContent("images/java.Dockerfile", fileTree)).toEqual(
		"java dockerfile",
	);
});

test("removeFile() removes a file from a folder", () => {
	const fileTree: FileTree = {
		"main.tf": "terraform content",
		images: {
			"java.Dockerfile": "java dockerfile",
			"python.Dockerfile": "python Dockerfile",
		},
	};
	const updatedFileTree = removeFile("images/python.Dockerfile", fileTree);
	const expectedFileTree = {
		"main.tf": "terraform content",
		images: {
			"java.Dockerfile": "java dockerfile",
		},
	};
	expect(updatedFileTree).toEqual(expectedFileTree);
	// Verify the original FileTree was not modified.
	expect((fileTree.images as FileTree)["python.Dockerfile"]).toEqual(
		"python Dockerfile",
	);
});

test("removeFile() removes a file from root", () => {
	const fileTree: FileTree = {
		"main.tf": "terraform content",
		images: {
			"java.Dockerfile": "java dockerfile",
			"python.Dockerfile": "python Dockerfile",
		},
	};
	const updatedFileTree = removeFile("main.tf", fileTree);
	const expectedFileTree = {
		images: {
			"java.Dockerfile": "java dockerfile",
			"python.Dockerfile": "python Dockerfile",
		},
	};
	expect(updatedFileTree).toEqual(expectedFileTree);
	// Verify the original FileTree was not modified.
	expect(fileTree["main.tf"]).toEqual("terraform content");
});

test("moveFile() moves a file from in file tree", () => {
	const fileTree: FileTree = {
		"main.tf": "terraform content",
		images: {
			"java.Dockerfile": "java dockerfile",
			"python.Dockerfile": "python Dockerfile",
		},
	};
	const updatedFileTree = moveFile(
		"images/java.Dockerfile",
		"other/java.Dockerfile",
		fileTree,
	);
	const expectedFileTree = {
		"main.tf": "terraform content",
		images: {
			"python.Dockerfile": "python Dockerfile",
		},
		other: {
			"java.Dockerfile": "java dockerfile",
		},
	};
	expect(updatedFileTree).toEqual(expectedFileTree);
	// Verify the original FileTree was not modified.
	expect(fileTree["main.tf"]).toEqual("terraform content");
});

test("existsFile() returns if there is or not a file", () => {
	const fileTree: FileTree = {
		"main.tf": "terraform content",
		images: { "java.Dockerfile": "java dockerfile" },
	};
	expect(existsFile("images/java.Dockerfile", fileTree)).toBeTruthy();
	expect(existsFile("no-existent-path", fileTree)).toBeFalsy();
});

test("isFolder() returns when a path is a folder or not", () => {
	const fileTree: FileTree = {
		"main.tf": "terraform content",
		images: { "java.Dockerfile": "java dockerfile" },
	};
	expect(isFolder("images", fileTree)).toBeTruthy();
	expect(isFolder("images/java.Dockerfile", fileTree)).toBeFalsy();
});

test("traverse() go trough all the file tree files", () => {
	const fileTree: FileTree = {
		"main.tf": "terraform content",
		images: { "java.Dockerfile": "java dockerfile" },
	};
	const filePaths: string[] = [];
	traverse(fileTree, (_content, _filename, fullPath) => {
		filePaths.push(fullPath);
	});
	const expectedFilePaths = ["images", "images/java.Dockerfile", "main.tf"];
	expect(filePaths).toEqual(expectedFilePaths);
});
