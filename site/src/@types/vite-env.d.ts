/// <reference types="vite/client" />

interface ViteTypeOptions {
   strictImportEnv: unknown
}

interface ImportMetaEnv {
  readonly VITE_DISABLE_EXTERNAL_LOGIN_PAGE: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}