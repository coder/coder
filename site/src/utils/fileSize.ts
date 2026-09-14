export const formatKiB = (bytes: number): string =>
	`${(bytes / 1024).toFixed(1)} KiB`;
