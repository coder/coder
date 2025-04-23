/// <reference types="vite/client" />

interface ViteTypeOptions {
   strictImportEnv: unknown
}

interface ImportMetaEnv {
  readonly VITE_IS_CI_BUILD: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}