// TEMPORARY, do not merge: must run before ./App installs the router.
import "./swipeDiagnostics";
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
	// TEMPORARY, do not merge: see swipeDiagServiceWorker in vite.config.mts.
	navigator.serviceWorker.register("/swipeDiagServiceWorker.js");
}

const root = createRoot(element);
root.render(<App />);
