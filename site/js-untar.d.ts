declare module "js-untar" {
  export interface File {
    name: string
    mode: string
    blob: Blob
    gid: number
    uid: number
    mtime: number
    gname: string
    uname: string
    type: "0" | "1" | "2" | "3" | "4" | "5" //https://en.wikipedia.org/wiki/Tar_(computing) on Type flag field
  }

  const Untar: (buffer: ArrayBuffer) => {
    then: (
      resolve?: (files: File[]) => void,
      reject?: () => Promise<void>,
      progress?: (file: File) => Promise<void>,
    ) => Promise<void>
  }

  export default Untar
}
