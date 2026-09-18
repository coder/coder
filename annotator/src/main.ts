import { mountAnnotator } from "./mountAnnotator";
import {
	type AnnotatorToHostMessage,
	isHostToAnnotatorMessage,
} from "./protocol";

/**
 * Entry point for the script the app proxy injects into proxied HTML. The
 * proxy sets `data-coder-origin` on the script tag so we only ever talk to
 * the dashboard that opened us, never to an arbitrary parent or opener.
 * The dashboard is either the embedding frame's parent or, for a popped
 * out preview, the window that opened this one.
 */
function bootstrap() {
	const script = document.currentScript;
	const hostOrigin =
		script instanceof HTMLScriptElement
			? script.dataset.coderOrigin
			: undefined;
	const hostWindow = window.parent !== window ? window.parent : window.opener;
	if (!hostOrigin || !hostWindow) {
		return;
	}

	const post = (message: AnnotatorToHostMessage) => {
		hostWindow.postMessage(message, hostOrigin);
	};

	const start = () => {
		const annotator = mountAnnotator({
			document,
			onSubmit: (submission) =>
				post({ type: "coder-annotator:submit", ...submission }),
			onStateChange: (state) =>
				post({ type: "coder-annotator:state", ...state }),
		});

		window.addEventListener("message", (event) => {
			if (
				event.origin !== hostOrigin ||
				event.source !== hostWindow ||
				!isHostToAnnotatorMessage(event.data)
			) {
				return;
			}
			switch (event.data.type) {
				case "coder-annotator:set-picking":
					annotator.setPicking(event.data.picking);
					break;
				case "coder-annotator:highlight":
					annotator.setHighlights(event.data.items);
					break;
				case "coder-annotator:clear-highlights":
					annotator.setHighlights([]);
					break;
			}
		});

		post({ type: "coder-annotator:ready" });
		post({ type: "coder-annotator:state", ...annotator.getState() });
	};

	if (document.readyState === "complete") {
		start();
	} else {
		// Wait for load rather than DOMContentLoaded so the embedding
		// dashboard sees the frame's load event before our ready message.
		window.addEventListener("load", start, { once: true });
	}
}

bootstrap();
