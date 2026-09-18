import { useCallback, useEffect, useRef, useState } from "react";

// Inline type declarations for the Web Speech API, which is not covered
// by TypeScript's built-in lib types in all environments.

interface SpeechRecognitionResultItem {
	readonly transcript: string;
	readonly confidence: number;
}

interface SpeechRecognitionResult {
	readonly length: number;
	readonly isFinal: boolean;
	item(index: number): SpeechRecognitionResultItem;
	[index: number]: SpeechRecognitionResultItem;
}

interface SpeechRecognitionResultList {
	readonly length: number;
	item(index: number): SpeechRecognitionResult;
	[index: number]: SpeechRecognitionResult;
}

interface SpeechRecognitionEvent extends Event {
	readonly resultIndex: number;
	readonly results: SpeechRecognitionResultList;
}

interface SpeechRecognitionErrorEvent extends Event {
	readonly error: string;
	readonly message: string;
}

interface SpeechRecognitionInstance extends EventTarget {
	lang: string;
	continuous: boolean;
	interimResults: boolean;
	onresult: ((event: SpeechRecognitionEvent) => void) | null;
	onerror: ((event: SpeechRecognitionErrorEvent) => void) | null;
	onend: (() => void) | null;
	start(): void;
	stop(): void;
	abort(): void;
}

interface SpeechRecognitionConstructor {
	new (): SpeechRecognitionInstance;
}

/**
 * Returns the SpeechRecognition constructor if the browser supports it,
 * or undefined otherwise.
 */
function getSpeechRecognitionCtor(): SpeechRecognitionConstructor | undefined {
	// The Web Speech API is available as SpeechRecognition in standards-
	// compliant browsers and as webkitSpeechRecognition in WebKit-based
	// browsers. We check both to maximise compatibility.
	const win = window as Window &
		typeof globalThis & {
			SpeechRecognition?: SpeechRecognitionConstructor;
			webkitSpeechRecognition?: SpeechRecognitionConstructor;
		};
	return win.SpeechRecognition ?? win.webkitSpeechRecognition;
}

/**
 * Standalone helper that can be called outside of React to check whether
 * the Web Speech API is available in the current browser.
 */
export function isSpeechRecognitionSupported(): boolean {
	return getSpeechRecognitionCtor() !== undefined;
}

export function useSpeechRecognition(): {
	isSupported: boolean;
	isRecording: boolean;
	transcript: string;
	error: string | null;
	start: () => void;
	stop: () => void;
	cancel: () => void;
} {
	const [isRecording, setIsRecording] = useState(false);
	const [transcript, setTranscript] = useState("");
	const [error, setError] = useState<string | null>(null);
	const recognitionRef = useRef<SpeechRecognitionInstance | null>(null);

	// Browser API availability is constant for the lifetime of the
	// page, so a lazy state initializer captures it once.
	const [ctorSnapshot] = useState(getSpeechRecognitionCtor);
	const isSupported = ctorSnapshot !== undefined;

	const start = useCallback(() => {
		if (!ctorSnapshot) {
			return;
		}

		// Tear down any lingering instance before creating a new one.
		if (recognitionRef.current) {
			recognitionRef.current.abort();
			recognitionRef.current = null;
		}

		setError(null);

		const recognition = new ctorSnapshot();
		recognition.lang = navigator.language;
		recognition.continuous = true;
		recognition.interimResults = true;

		// We accumulate finalized text in a local variable so that the
		// onresult handler can build the full transcript from both final
		// and interim segments without depending on React state timing.
		let finalizedText = "";

		recognition.onresult = (event: SpeechRecognitionEvent) => {
			let interim = "";
			for (let i = event.resultIndex; i < event.results.length; i++) {
				const result = event.results[i];
				if (result.isFinal) {
					finalizedText += result[0].transcript;
				} else {
					interim += result[0].transcript;
				}
			}
			setTranscript(finalizedText + interim);
		};

		recognition.onerror = (event: SpeechRecognitionErrorEvent) => {
			if (recognitionRef.current !== recognition) return;
			setError(event.error);
			setIsRecording(false);
			recognitionRef.current = null;
		};

		recognition.onend = () => {
			if (recognitionRef.current !== recognition) return;
			setIsRecording(false);
			recognitionRef.current = null;
		};

		recognitionRef.current = recognition;
		setTranscript("");
		setIsRecording(true);
		recognition.start();
	}, [ctorSnapshot]);

	const stop = useCallback(() => {
		// stop() lets the browser deliver any remaining final results
		// before firing the onend event.
		if (recognitionRef.current) {
			recognitionRef.current.stop();
			recognitionRef.current = null;
		}
		setIsRecording(false);
	}, []);

	const cancel = useCallback(() => {
		// abort() discards any pending audio and results immediately.
		if (recognitionRef.current) {
			recognitionRef.current.abort();
			recognitionRef.current = null;
		}
		setIsRecording(false);
		setTranscript("");
		setError(null);
	}, []);

	useEffect(() => {
		return () => {
			if (recognitionRef.current) {
				recognitionRef.current.abort();
				recognitionRef.current = null;
			}
		};
	}, []);

	return { isSupported, isRecording, transcript, error, start, stop, cancel };
}
