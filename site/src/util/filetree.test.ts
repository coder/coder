import {
  existsFile,
  FileTree,
  getFileContent,
  isFolder,
  moveFile,
  removeFile,
  setFile,
  traverse,
} from "./filetree"

test("setFile() set file into the file tree", () => {
  let fileTree: FileTree = {
    "main.tf": "terraform",
    images: { "java.Dockerfile": "java dockerfile" },
  }
  fileTree = setFile("images/python.Dockerfile", "python dockerfile", fileTree)
  expect((fileTree.images as FileTree)["python.Dockerfile"]).toEqual(
    "python dockerfile",
  )
})

test("getFileContent() return the file content from the file tree", () => {
  const fileTree: FileTree = {
    "main.tf": "terraform content",
    images: { "java.Dockerfile": "java dockerfile" },
  }
  expect(getFileContent("images/java.Dockerfile", fileTree)).toEqual(
    "java dockerfile",
  )
})

test("removeFile() removes a file from the file tree", () => {
  let fileTree: FileTree = {
    "main.tf": "terraform content",
    images: {
      "java.Dockerfile": "java dockerfile",
      "python.Dockerfile": "python Dockerfile",
    },
  }
  fileTree = removeFile("images/python.Dockerfile", fileTree)
  const expectedFileTree = {
    "main.tf": "terraform content",
    images: {
      "java.Dockerfile": "java dockerfile",
    },
  }
  expect(expectedFileTree).toEqual(fileTree)
})

test("moveFile() moves a file from in file tree", () => {
  let fileTree: FileTree = {
    "main.tf": "terraform content",
    images: {
      "java.Dockerfile": "java dockerfile",
      "python.Dockerfile": "python Dockerfile",
    },
  }
  fileTree = moveFile(
    "images/java.Dockerfile",
    "other/java.Dockerfile",
    fileTree,
  )
  const expectedFileTree = {
    "main.tf": "terraform content",
    images: {
      "python.Dockerfile": "python Dockerfile",
    },
    other: {
      "java.Dockerfile": "java dockerfile",
    },
  }
  expect(fileTree).toEqual(expectedFileTree)
})

test("existsFile() returns if there is or not a file", () => {
  const fileTree: FileTree = {
    "main.tf": "terraform content",
    images: { "java.Dockerfile": "java dockerfile" },
  }
  expect(existsFile("images/java.Dockerfile", fileTree)).toBeTruthy()
  expect(existsFile("no-existent-path", fileTree)).toBeFalsy()
})

test("isFolder() returns when a path is a folder or not", () => {
  const fileTree: FileTree = {
    "main.tf": "terraform content",
    images: { "java.Dockerfile": "java dockerfile" },
  }
  expect(isFolder("images", fileTree)).toBeTruthy()
  expect(isFolder("images/java.Dockerfile", fileTree)).toBeFalsy()
})

test("traverse() go trough all the file tree files", () => {
  const fileTree: FileTree = {
    "main.tf": "terraform content",
    images: { "java.Dockerfile": "java dockerfile" },
  }
  const filePaths: string[] = []
  traverse(fileTree, (_content, _filename, fullPath) => {
    filePaths.push(fullPath)
  })
  const expectedFilePaths = ["main.tf", "images", "images/java.Dockerfile"]
  expect(filePaths).toEqual(expectedFilePaths)
})
