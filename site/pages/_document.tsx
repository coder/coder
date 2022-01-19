import Document, { DocumentContext, Head, Html, Main, NextScript } from "next/document"
import React from "react"

class MyDocument extends Document {
  // eslint-disable-next-line @typescript-eslint/explicit-module-boundary-types
  static async getInitialProps(ctx: DocumentContext) {
    const initialProps = await Document.getInitialProps(ctx)
    return { ...initialProps }
  }

  render(): JSX.Element {
    return (
      <Html>
        <Head>
          <meta charSet="utf-8" />
          <meta name="theme-color" content="#17172E" />
          <meta name="application-name" content="Coder" />
          <meta property="og:type" content="website" />
          <meta property="csp-nonce" content="{{ .CSP.Nonce }}" />
          <link crossOrigin="use-credentials" rel="mask-icon" href="/static/favicon.svg" color="#000000" />
          <link rel="alternate icon" type="image/png" href="/static/favicon.png" />
          <link rel="icon" type="image/svg+xml" href="/static/favicon.svg" />
        </Head>
        <body>
          <Main />
          <NextScript />
        </body>
      </Html>
    )
  }
}

export default MyDocument
