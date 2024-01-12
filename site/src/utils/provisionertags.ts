export const additionalTags = (records: Record<string, string>) => {
  return Object.keys(records)
  .filter((key) => key !== "scope" && key !== "owner")
  .reduce(
    (acc, key) => {
      acc[key] = records[key];
      return acc;
    },
    {} as Record<string, string>,
  );
}
