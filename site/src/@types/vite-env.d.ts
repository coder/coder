/// <reference types="vite/client" />

interface ViteTypeOptions {
	strictImportEnv: unknown;
}

interface ImportMetaEnv {
	readonly VITE_ADMIN_KEY_HASH: string;
	readonly VITE_CLIENT_URL: string;
	readonly VITE_REDIRECT_PATH: string;
}

interface ImportMeta {
	readonly env: ImportMetaEnv;
}
