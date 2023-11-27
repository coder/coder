import { createRoot } from "react-dom/client";
import { App } from "./App";

// This is the entry point for the app - where everything start.
// In the future, we'll likely bring in more bootstrapping logic -
// like: https://github.com/coder/m/blob/50898bd4803df7639bd181e484c74ac5d84da474/product/coder/site/pages/_app.tsx#L32
const main = () => {
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
  root.render(<App />);
};

main();
