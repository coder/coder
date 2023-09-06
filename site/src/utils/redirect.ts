/**
 * Creates a url containing a page to navigate to now, and embedding another
 * URL in the query string so you can return to it later.
 * @param navigateTo page to navigate to now (by default, /login)
 * @param returnTo page to redirect to later (for instance, after logging in)
 * @returns URL containing a redirect query parameter
 */
export const embedRedirect = (
  returnTo: string,
  navigateTo = "/login",
): string => `${navigateTo}?redirect=${encodeURIComponent(returnTo)}`;

/**
 * Retrieves a url from the query string of the current URL
 * @param search the query string in the current URL
 * @returns the URL to redirect to
 */
export const retrieveRedirect = (search: string): string => {
  const defaultRedirect = "/";
  const searchParams = new URLSearchParams(search);
  const redirect = searchParams.get("redirect");
  return redirect ? redirect : defaultRedirect;
};
