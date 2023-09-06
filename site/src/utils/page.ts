export const pageTitle = (prefix: string | string[]): string => {
  const title = Array.isArray(prefix) ? prefix.join(" · ") : prefix;
  return `${title} - Coder`;
};
