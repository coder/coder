export const prepareQuery = (query?: string) => {
  return query?.trim().replace(/  +/g, " ");
};
