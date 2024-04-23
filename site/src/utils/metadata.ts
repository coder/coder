export const getMetadataAsJSON = <T extends NonNullable<unknown>>(
  property: string,
): T | undefined => {
  const metadata = document.querySelector(`meta[property=${property}]`);

  if (metadata) {
    const rawContent = metadata.getAttribute("content");

    if (rawContent) {
      try {
        return JSON.parse(rawContent);
      } catch (err) {
        // In development, the metadata is always going to be empty; error is
        // only a concern for production
        if (process.env.NODE_ENV === "production") {
          console.warn(`Failed to parse ${property} metadata. Error message:`);
          console.warn(err);
        }
      }
    }
  }

  return undefined;
};
