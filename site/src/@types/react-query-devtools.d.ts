// extending the global window interface so we can conditionally
// show our react query devtools
declare global {
	interface Window {
		toggleDevtools: () => void;
	}
}

export {};
