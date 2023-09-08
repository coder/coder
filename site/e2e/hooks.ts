import { Page } from "@playwright/test";

export const beforeCoderTest = async (page: Page) => {
  // eslint-disable-next-line no-console -- Show everything that was printed with console.log()
  page.on("console", (msg) => console.log("[onConsole] " + msg.text()));

  page.on("request", (request) => {
    if (!isApiCall(request.url())) {
      return;
    }

    // eslint-disable-next-line no-console -- Log HTTP requests for debugging purposes
    console.log(
      `[onRequest] method=${request.method()} url=${request.url()} postData=${
        request.postData() ? request.postData() : ""
      }`,
    );
  });
  page.on("response", async (response) => {
    if (!isApiCall(response.url())) {
      return;
    }

    const shouldLogResponse =
      !response.url().endsWith("/api/v2/deployment/config") &&
      !response.url().endsWith("/api/v2/debug/health");

    let responseText = "";
    try {
      if (shouldLogResponse) {
        const buffer = await response.body();
        responseText = buffer.toString("utf-8");
        responseText = responseText.replace(/\n$/g, "");
      } else {
        responseText = "skipped...";
      }
    } catch (error) {
      responseText = "not_available";
    }

    // eslint-disable-next-line no-console -- Log HTTP requests for debugging purposes
    console.log(
      `[onResponse] url=${response.url()} status=${response.status()} body=${responseText}`,
    );
  });
};

const isApiCall = (urlString: string): boolean => {
  const url = new URL(urlString);
  const apiPath = "/api/v2";

  return url.pathname.startsWith(apiPath);
};
