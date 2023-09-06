import { embedRedirect, retrieveRedirect } from "./redirect";

describe("redirect helper functions", () => {
  describe("embedRedirect", () => {
    it("embeds the page to return to in the URL", () => {
      const result = embedRedirect("/workspaces", "/page");
      expect(result).toEqual("/page?redirect=%2Fworkspaces");
    });
    it("defaults to navigating to the login page", () => {
      const result = embedRedirect("/workspaces");
      expect(result).toEqual("/login?redirect=%2Fworkspaces");
    });
  });
  describe("retrieveRedirect", () => {
    it("retrieves the page to return to from the URL", () => {
      const result = retrieveRedirect("?redirect=%2Fworkspaces");
      expect(result).toEqual("/workspaces");
    });
  });
});
