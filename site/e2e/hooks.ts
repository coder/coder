import type { BrowserContext, Page } from "@playwright/test";
import http from "http";
import { coderPort, gitAuth } from "./constants";

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
      !response.url().endsWith("/api/v2/debug/health?force=false");

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

export const resetExternalAuthKey = async (context: BrowserContext) => {
  // Find the session token so we can destroy the external auth link between tests, to ensure valid authentication happens each time.
  const cookies = await context.cookies();
  const sessionCookie = cookies.find((c) => c.name === "coder_session_token");
  const options = {
    method: "DELETE",
    hostname: "127.0.0.1",
    port: coderPort,
    path: `/api/v2/external-auth/${gitAuth.webProvider}?coder_session_token=${sessionCookie?.value}`,
  };

  const req = http.request(options, (res) => {
    let data = "";
    res.on("data", (chunk) => {
      data += chunk;
    });

    res.on("end", () => {
      // Both 200 (key deleted successfully) and 500 (key was not found) are valid responses.
      if (res.statusCode !== 200 && res.statusCode !== 500) {
        console.error("failed to delete external auth link", data);
        throw new Error(
          `failed to delete external auth link: HTTP response ${res.statusCode}`,
        );
      }
    });
  });

  req.on("error", (err) => {
    throw err.message;
  });

  req.end();
};

const isApiCall = (urlString: string): boolean => {
  const url = new URL(urlString);
  const apiPath = "/api/v2";

  return url.pathname.startsWith(apiPath);
};
