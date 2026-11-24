import { act, renderHook } from "@testing-library/react";
import {
	isSpeechRecognitionSupported,
	useSpeechRecognition,
} from "./useSpeechRecognition";

// ---------------------------------------------------------------------------
// Minimal mock for the Web Speech API SpeechRecognition class.
// ---------------------------------------------------------------------------

type ResultHandler = (event: {
	resultIndex: number;
	results: {
		length: number;
		[i: number]: {
			isFinal: boolean;
			0: { transcript: string; confidence: number };
		};
	};
}) => void;

class MockSpeechRecognition {
	lang = "";
	continuous = false;
	interimResults = false;
	onresult: ResultHandler | null = null;
	onerror: ((event: { error: string; message: string }) => void) | null = null;
	onend: (() => void) | null = null;

	start = vi.fn();
	stop = vi.fn(() => {
		// Browser fires onend after stop.
		this.onend?.();
	});
	abort = vi.fn(() => {
		this.onend?.();
	});
}

let lastInstance: MockSpeechRecognition | null = null;

function installMock() {
	lastInstance = null;
	// Use a real class so `new Ctor()` works — vi.fn() arrow
	// functions are not constructable.
	class Ctor extends MockSpeechRecognition {
		constructor() {
			super();
			// oxlint-disable-next-line typescript/no-this-alias -- the constructor must record the created instance for the test to access.
			lastInstance = this;
		}
	}
	Object.assign(window, { SpeechRecognition: Ctor });
	return Ctor;
}

function removeMock() {
	Object.assign(window, {
		SpeechRecognition: undefined,
		webkitSpeechRecognition: undefined,
	});
	lastInstance = null;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

afterEach(() => {
	removeMock();
});

describe("isSpeechRecognitionSupported", () => {
	it("returns false when API is not available", () => {
		removeMock();
		expect(isSpeechRecognitionSupported()).toBe(false);
	});

	it("returns true when SpeechRecognition is on window", () => {
		installMock();
		expect(isSpeechRecognitionSupported()).toBe(true);
	});
});

describe("useSpeechRecognition", () => {
	it("reports isSupported=false when API is missing", () => {
		removeMock();
		const { result } = renderHook(() => useSpeechRecognition());
		expect(result.current.isSupported).toBe(false);
		expect(result.current.isRecording).toBe(false);
	});

	it("reports isSupported=true when API is present", () => {
		installMock();
		const { result } = renderHook(() => useSpeechRecognition());
		expect(result.current.isSupported).toBe(true);
	});

	it("starts recording and sets isRecording", () => {
		installMock();
		const { result } = renderHook(() => useSpeechRecognition());

		act(() => {
			result.current.start(vi.fn());
		});

		expect(result.current.isRecording).toBe(true);
		expect(lastInstance?.start).toHaveBeenCalled();
		expect(lastInstance?.continuous).toBe(true);
		expect(lastInstance?.interimResults).toBe(true);
	});

	it("accumulates transcript from result events", () => {
		installMock();
		const { result } = renderHook(() => useSpeechRecognition());
		const onTranscript = vi.fn();

		act(() => {
			result.current.start(onTranscript);
		});

		// Simulate an interim result.
		act(() => {
			lastInstance?.onresult?.({
				resultIndex: 0,
				results: {
					length: 1,
					0: { isFinal: false, 0: { transcript: "hello", confidence: 0.9 } },
				},
			});
		});
		expect(onTranscript).toHaveBeenLastCalledWith("hello");

		// Simulate it becoming final + a new interim.
		act(() => {
			lastInstance?.onresult?.({
				resultIndex: 0,
				results: {
					length: 2,
					0: { isFinal: true, 0: { transcript: "hello ", confidence: 0.99 } },
					1: { isFinal: false, 0: { transcript: "world", confidence: 0.8 } },
				},
			});
		});
		expect(onTranscript).toHaveBeenLastCalledWith("hello world");
	});

	it("stop() sets isRecording=false and ignores later results", () => {
		installMock();
		const { result } = renderHook(() => useSpeechRecognition());
		const onTranscript = vi.fn();

		act(() => {
			result.current.start(onTranscript);
		});

		act(() => {
			result.current.stop();
		});

		act(() => {
			lastInstance?.onresult?.({
				resultIndex: 0,
				results: {
					length: 1,
					0: { isFinal: true, 0: { transcript: "late", confidence: 1 } },
				},
			});
		});

		expect(result.current.isRecording).toBe(false);
		expect(onTranscript).not.toHaveBeenCalled();
		expect(lastInstance?.stop).toHaveBeenCalled();
	});

	it("cancel() aborts and sets isRecording=false", () => {
		installMock();
		const { result } = renderHook(() => useSpeechRecognition());

		act(() => {
			result.current.start(vi.fn());
		});

		act(() => {
			result.current.cancel();
		});

		expect(result.current.isRecording).toBe(false);
		expect(lastInstance?.abort).toHaveBeenCalled();
	});

	it("cleans up on onerror", () => {
		installMock();
		const { result } = renderHook(() => useSpeechRecognition());

		act(() => {
			result.current.start(vi.fn());
		});
		expect(result.current.isRecording).toBe(true);

		act(() => {
			lastInstance?.onerror?.({ error: "not-allowed", message: "" });
		});
		expect(result.current.isRecording).toBe(false);
	});

	it("start() aborts a previous instance", () => {
		installMock();
		const { result } = renderHook(() => useSpeechRecognition());

		act(() => {
			result.current.start(vi.fn());
		});
		const first = lastInstance;

		act(() => {
			result.current.start(vi.fn());
		});

		expect(first?.abort).toHaveBeenCalled();
		expect(lastInstance).not.toBe(first);
	});

	it("start() ignores onend from a previously aborted instance", () => {
		installMock();
		const { result } = renderHook(() => useSpeechRecognition());

		// Start recording — creates the first instance.
		act(() => {
			result.current.start(vi.fn());
		});
		const first = lastInstance!;

		// Override abort so it does NOT fire onend synchronously,
		// simulating the async browser behaviour.
		first.abort = vi.fn();

		// Start recording again — creates a second instance and aborts
		// the first.
		act(() => {
			result.current.start(vi.fn());
		});
		const second = lastInstance!;

		expect(first.abort).toHaveBeenCalled();
		expect(result.current.isRecording).toBe(true);

		// Simulate the OLD instance's async onend firing late.
		act(() => {
			first.onend?.();
		});

		// The old onend must be ignored — recording is still active.
		expect(result.current.isRecording).toBe(true);
		expect(lastInstance).toBe(second);
	});

	it("exposes error from onerror event", () => {
		installMock();
		const { result } = renderHook(() => useSpeechRecognition());

		expect(result.current.error).toBeNull();

		act(() => {
			result.current.start(vi.fn());
		});

		act(() => {
			lastInstance?.onerror?.({
				error: "not-allowed",
				message: "Permission denied",
			});
		});

		expect(result.current.error).toBe("not-allowed");
		expect(result.current.isRecording).toBe(false);
	});

	it("start() clears previous error", () => {
		installMock();
		const { result } = renderHook(() => useSpeechRecognition());

		act(() => {
			result.current.start(vi.fn());
		});

		act(() => {
			lastInstance?.onerror?.({
				error: "not-allowed",
				message: "Permission denied",
			});
		});
		expect(result.current.error).toBe("not-allowed");

		act(() => {
			result.current.start(vi.fn());
		});

		expect(result.current.error).toBeNull();
		expect(result.current.isRecording).toBe(true);
	});

	it("cancel() clears error", () => {
		installMock();
		const { result } = renderHook(() => useSpeechRecognition());

		act(() => {
			result.current.start(vi.fn());
		});

		act(() => {
			lastInstance?.onerror?.({
				error: "not-allowed",
				message: "Permission denied",
			});
		});
		expect(result.current.error).toBe("not-allowed");

		act(() => {
			result.current.cancel();
		});

		expect(result.current.error).toBeNull();
	});
});
