import { createRoot } from "react-dom/client";
import "./index.css";
import { App } from "./App";
import { registerStaleChunkReload } from "./utils/staleChunkReload";

console.info(`      -#######          +######-      ########+       ##########  ########+.      ###########
   +#####--######    +#####--#####+   ############    ##########  ####+++#####-   ###########
  ####-      -####  ####-      #####  ####     ####+  ####        ####    .####   ###########
 .####              ####        ####  ####      ####  #########   ####...+##+     ###########
  ####.      .####  ####       +####  ####     +####  ####        ####+#######    ###########
   #####-  -#####    ######..######   ############-   ##########  ####    #####   ###########
     .########+        -########.     #########+      ##########  ####    .####   ###########
`);

registerStaleChunkReload();

const element = document.getElementById("root");
if (element === null) {
	throw new Error("root element is null");
}

// The service worker handles push notifications.
if ("serviceWorker" in navigator) {
	navigator.serviceWorker.register("/serviceWorker.js");
}

const root = createRoot(element);
root.render(<App />);
