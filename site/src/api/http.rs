//! Thin wrappers around gloo-net that attach the CSRF token
//! header required by the Coder API.

use gloo_net::http::{Request, RequestBuilder};

/// Hard-coded CSRF token matching the cookie set in index.html.
/// In production the Go server generates both dynamically; for
/// local development we use a static pair.
const CSRF_TOKEN: &str =
    "KNKvagCBEHZK7ihe2t7fj6VeJ0UyTDco1yVUJE8N06oNqxLu5Zx1vRxZbgfC0mJJgeGkVjgs08mgPbcWPBkZ1A==";

/// Start a GET request with the CSRF header pre-attached.
pub fn get(url: &str) -> RequestBuilder {
    Request::get(url).header("X-CSRF-TOKEN", CSRF_TOKEN)
}

/// Start a POST request with Content-Type JSON and CSRF header.
pub fn post(url: &str) -> RequestBuilder {
    Request::post(url)
        .header("X-CSRF-TOKEN", CSRF_TOKEN)
        .header("Content-Type", "application/json")
}
