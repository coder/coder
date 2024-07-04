import { createRoot } from "react-dom/client";
import { tryLoadAndStartRecorder } from "@alwaysmeticulous/recorder-loader";
import { App } from "./App";

console.info(`    ▄█▀    ▀█▄
     ▄▄ ▀▀▀  █▌   ██▀▀█▄          ▐█
 ▄▄██▀▀█▄▄▄  ██  ██      █▀▀█ ▐█▀▀██ ▄█▀▀█ █▀▀
█▌   ▄▌   ▐█ █▌  ▀█▄▄▄█▌ █  █ ▐█  ██ ██▀▀  █
     ██████▀▄█    ▀▀▀▀   ▀▀▀▀  ▀▀▀▀▀  ▀▀▀▀ ▀
`);

const element = document.getElementById("root");
if (element === null) {
  throw new Error("root element is null");
}

const root = createRoot(element);
async function startApp() {
  // Record all sessions on localhost, staging stacks and preview URLs
  if (isInternal()) {
    // Start the Meticulous recorder before you initialise your app.
    // Note: all errors are caught and logged, so no need to surround with try/catch
    await tryLoadAndStartRecorder({
      projectId: "Y4uHy1qs0B660xxUdrkLPkazUMPr6OuTqYEnShaR",
      isProduction: false,
    });
  }

  root.render(<App />);
}

function isInternal() {
  return (
    window.location.hostname.indexOf("dev.coder.com") > -1 ||
    window.location.hostname.indexOf("localhost") > -1 ||
    window.location.hostname.indexOf("127.0.0.1") > -1
  );
}

startApp();
