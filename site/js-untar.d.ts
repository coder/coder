declare module "js-untar" {
  interface File {
    name: string
    blob: Blob
  }

  const Untar: (buffer: ArrayBuffer) => {
    then: (
      resolve?: () => Promise<void>,
      reject?: () => Promise<void>,
      progress: (file: File) => Promise<void>,
    ) => Promise<void>
  }

  export default Untar
}
