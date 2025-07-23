import { useEffect } from "react";

const CursorSparkle = (): JSX.Element | null => {
	useEffect(() => {
		const createSparkle = (e: MouseEvent) => {
			const sparkle = document.createElement("div");
			sparkle.className = "cursor-sparkle";

			// Set the position to the mouse position
			sparkle.style.position = "fixed";
			sparkle.style.left = `${e.clientX}px`;
			sparkle.style.top = `${e.clientY}px`;

			// Create random size for variety
			const size = Math.random() * 10 + 5;
			sparkle.style.width = `${size}px`;
			sparkle.style.height = `${size}px`;

			// Star shape using a rotated square
			sparkle.style.backgroundColor = "transparent";
			sparkle.style.backgroundImage =
				"radial-gradient(white, rgba(255, 255, 255, 0))";
			sparkle.style.boxShadow = "0 0 5px 2px rgba(255, 255, 0, 0.7)";
			sparkle.style.borderRadius = "50%";

			// Animation to fade out and grow
			sparkle.style.animation = "sparkle 0.8s forwards";
			sparkle.style.pointerEvents = "none"; // Prevent the sparkles from interfering with mouse events

			// Random rotation for variety
			sparkle.style.transform = `rotate(${Math.random() * 360}deg)`;

			// Add to body and remove after animation completes
			document.body.appendChild(sparkle);
			setTimeout(() => {
				if (sparkle.parentNode) {
					sparkle.parentNode.removeChild(sparkle);
				}
			}, 800); // Same time as the animation duration
		};

		// Throttle to reduce performance impact
		let lastSparkleTime = 0;
		const throttleDelay = 25; // ms between sparkles

		const handleMouseMove = (e: MouseEvent) => {
			const now = Date.now();
			if (now - lastSparkleTime >= throttleDelay) {
				lastSparkleTime = now;
				createSparkle(e);
			}
		};

		// Add event listener
		document.addEventListener("mousemove", handleMouseMove);

		// Cleanup
		return () => {
			document.removeEventListener("mousemove", handleMouseMove);

			// Remove any remaining sparkles
			const sparkles = document.querySelectorAll(".cursor-sparkle");
			for (const sparkle of sparkles) {
				if (sparkle.parentNode) {
					sparkle.parentNode.removeChild(sparkle);
				}
			}
		};
	}, []);

	return null; // This component doesn't render anything visible
};

export default CursorSparkle;
