import { ChakraProvider, extendTheme } from "@chakra-ui/react";
import type { AppProps } from "next/app";
import Head from "next/head";

const theme = extendTheme({
	styles: {
		global: {
			body: {
				bg: "gray.50",
			},
		},
	},
});

const MyApp: React.FC<AppProps> = ({ Component, pageProps }) => {
	return (
		<>
			<Head>
				<link
					rel="alternate icon"
					type="image/png"
					href="/favicon-light.png"
					media="(prefers-color-scheme: dark)"
				/>
				<link
					rel="icon"
					type="image/svg+xml"
					href="/favicon-light.svg"
					media="(prefers-color-scheme: dark)"
				/>
				<link
					rel="alternate icon"
					type="image/png"
					href="/favicon-dark.png"
					media="(prefers-color-scheme: light)"
				/>
				<link
					rel="icon"
					type="image/svg+xml"
					href="/favicon-dark.svg"
					media="(prefers-color-scheme: light)"
				/>
				<link rel="mask-icon" href="/favicon-dark.svg" color="#090B0B" />
			</Head>
			<ChakraProvider theme={theme}>
				<Component {...pageProps} />
			</ChakraProvider>
		</>
	);
};

export default MyApp;
