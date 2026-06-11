namespace React {
	export interface CSSProperties {
		[customProp: `--${string}`]: string | number | undefined;
	}
}
