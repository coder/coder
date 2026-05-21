# code-server

[code-server](https://github.com/coder/code-server) is our supported method of running VS Code in the web browser.

![code-server in a workspace](../../images/code-server-ide.png)

## Differences between code-server and VS Code Web

Some of the key differences between code-server and VS Code Web are:

| Feature                  | code-server                                                                 | VS Code Web                                                       |
|--------------------------|-----------------------------------------------------------------------------|-------------------------------------------------------------------|
| Authentication           | Optional login form                                                         | No built-in auth                                                  |
| Built-in proxy           | Includes development proxy (not needed with Coder)                          | No built-in development proxy                                     |
| Clipboard integration    | Supports piping text from terminal (similar to `xclip`)                     | More limited                                                      |
| Display languages        | Supports language pack extensions                                           | Limited language support                                          |
| File operations          | Options to disable downloads and uploads                                    | No built-in restrictions                                          |
| Health endpoint          | Provides `/healthz` endpoint                                                | Limited health monitoring                                         |
| Marketplace              | Open VSX by default, configurable via flags/env vars                        | Uses Microsoft marketplace; modify `product.json` to use your own |
| Path-based routing       | Has fixes for state collisions when used path-based                         | May have issues with path-based routing in certain configurations |
| Proposed API             | Always enabled for all extensions                                           | Only Microsoft extensions without configuration                   |
| Proxy integration        | Integrates with Coder's proxy for ports panel                               | Integration is more limited                                       |
| Sourcemaps               | Loads locally                                                               | Uses CDN                                                          |
| Telemetry                | Configurable endpoint                                                       | Does not allow a configurable endpoint                            |
| Terminal access to files | You can use a terminal outside of the integrated one to interact with files | Limited to integrated terminal access                             |
| User settings            | Stored on remote disk                                                       | Stored in browser                                                 |
| Web views                | Self-contained                                                              | Uses Microsoft CDN                                                |

For more information about code-server, visit the [code-server FAQ](https://coder.com/docs/code-server/FAQ).
