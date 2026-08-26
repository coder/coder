import { createRoot } from "react-dom/client";
import "./index.css";
import { App } from "./App";
import { defineStorageKey, integerCodec } from "./storage";

/** Timestamp of the last chunk-preload-failure reload (see below). */
const preloadReloadStorage = defineStorageKey<number | null>({
	key: "preload-reload",
	codec: integerCodec,
	defaultValue: null,
	area: "session",
});

console.info(`      -#######          +######-      ########+       ##########  ########+.      ###########
   +#####--######    +#####--#####+   ############    ##########  ####+++#####-   ###########
  ####-      -####  ####-      #####  ####     ####+  ####        ####    .####   ###########
 .####              ####        ####  ####      ####  #########   ####...+##+     ###########
  ####.      .####  ####       +####  ####     +####  ####        ####+#######    ###########
   #####-  -#####    ######..######   ############-   ##########  ####    #####   ###########
     .########+        -########.     #########+      ##########  ####    .####   ###########
`);

// After a redeploy, the SPA still holds old chunk filenames in memory.
// Navigating to a lazy-loaded route will try to fetch a chunk that no
// longer exists (the content hash changed), crashing the app. Vite fires
// this event whenever a dynamic-import preload fails. We catch it and
// silently reload so the browser fetches a fresh index.html with the new
// chunk names. A sessionStorage guard prevents infinite reload loops.
window.addEventListener("vite:preloadError", () => {
	const last = preloadReloadStorage.get();
	const now = Date.now();
	if (last === null || now - last > 10_000) {
		// Reload only when the guard actually persisted; otherwise a
		// persistent preload error would reload forever.
		if (preloadReloadStorage.set(now).ok) {
			location.reload();
		}
	}
});

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
