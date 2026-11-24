import type { QueryClient } from "react-query";

export function prepareQuery(query: string): string;
export function prepareQuery(query: undefined): undefined;
export function prepareQuery(query: string | undefined): string | undefined;
export function prepareQuery(query?: string): string | undefined {
	return query?.trim().replace(/ {2,}/g, " ");
}

/** The slice of `QueryClient` the option loaders depend on. */
export type OptionsQueryClient = Pick<QueryClient, "fetchQuery">;
