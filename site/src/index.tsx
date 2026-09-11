import { getDefaultStore } from "jotai/vanilla";
import { atomWithStorage } from "jotai/vanilla/utils";
import { createRoot } from "react-dom/client";
import "./index.css";
import { App } from "./App";
import { createCodecStorage, integerCodec } from "./storage";

const preloadReloadAtom = atomWithStorage<number | null>(
	"preload-reload",
	null,
	createCodecStorage<number | null>(() => sessionStorage, integerCodec),
	{ getOnInit: true },
);

const store = getDefaultStore();

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
	const last = store.get(preloadReloadAtom);
	const now = Date.now();
	if (last === null || now - last > 10_000) {
		store.set(preloadReloadAtom, now);
		try {
			if (sessionStorage.getItem("preload-reload") === String(now)) {
				location.reload();
			}
		} catch {
			// Reloading without a persisted guard could cause a loop.
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
