import type { FC, PropsWithChildren } from "react";

export type WorkerInitializationRenderOptions = {
	theme?: {
		light?: string;
		dark?: string;
	};
};

export type WorkerPoolOptions = {
	poolSize?: number;
	workerFactory?: () => Worker;
};

export type SupportedLanguages = string;

export const WorkerPoolContextProvider: FC<PropsWithChildren> = ({
	children,
}) => <>{children}</>;

export const FileDiff: FC = () => null;

export const File: FC = () => null;
