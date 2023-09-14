// eslint-disable-next-line @typescript-eslint/no-explicit-any -- It can be any
export const getMetadataAsJSON = <T extends Record<string, any>>(
  property: string,
): T | undefined => {
  const appearance = document.querySelector(`meta[property=${property}]`);
  if (appearance) {
    const rawContent = appearance.getAttribute("content");
    try {
      return JSON.parse(rawContent as string);
    } catch (ex) {
      throw new Error(`Failed to parse ${property} metadata`);
    }
  }
};
