export function prepareQuery(query: undefined): undefined;
export function prepareQuery(query: string): string;
export function prepareQuery(query?: string): string | undefined {
  return query?.trim().replace(/  +/g, " ");
}
